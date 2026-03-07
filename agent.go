package yac

import (
	"context"
	"encoding/json"
	"fmt"
)

// maxToolRounds limits tool-use round trips to prevent infinite loops.
const maxToolRounds = 10

// Adapter is the interface to a backend LLM.
// Any backend (OpenAI, Anthropic, Ollama, etc.) implements this.
type Adapter interface {
	// SendMessage sends a chat completion request and returns the
	// model's response. The context supports cancellation and timeouts.
	SendMessage(ctx context.Context, req *ChatRequest) (Message, error)
}

// ChatRequest holds the parameters for a single chat completion call.
type ChatRequest struct {
	// Messages is the conversation history to send.
	Messages []Message

	// Tools are the tool definitions available to the model this turn.
	Tools []*Tool

	// ToolChoice controls tool usage. Values: "auto", "none",
	// "required", or a map for forcing a specific tool.
	// Nil means API default (usually "auto").
	ToolChoice any
}

// Agent is the core unit of work in yac.
type Agent struct {
	// ContextLength is the maximum number of tokens the agent's
	// backing model can handle in a single request.
	ContextLength int

	// Adapter connects the agent to a real LLM backend.
	Adapter Adapter

	// SystemPrompt is called before every request to produce the
	// system message prepended to the conversation. Use the provided
	// helpers to construct it:
	//
	//   StaticPrompt("You are helpful.")        — constant string
	//   TemplatePrompt(tmpl, dataFn)            — re-rendered each call
	//   CachedPrompt(TemplatePrompt(tmpl, fn))  — rendered once, then cached
	//
	// If nil, no system message is included.
	SystemPrompt func() string

	// Tools is the default set of tools available to the agent.
	// Can be overridden per-turn with WithTools.
	Tools []*Tool

	// Messages is the conversation history for this agent.
	Messages []Message
}

// Send sends a user message and returns the assistant's final response.
// It handles the full tool-use loop: if the model calls tools, Send
// executes them, feeds the results back, and repeats until the model
// produces a text response.
//
// Options control per-turn behavior:
//
//	reply, _ := agent.Send(ctx, "What's the weather?")                       // defaults
//	reply, _ := agent.Send(ctx, "Calculate", yac.WithTools(calcTool))        // override tools
//	reply, _ := agent.Send(ctx, "Get weather", yac.ForceToolUse("weather"))  // force tool
//	reply, _ := agent.Send(ctx, "Just chat", yac.WithToolChoice(yac.None))   // no tools
func (a *Agent) Send(ctx context.Context, content string, opts ...SendOption) (Message, error) {
	cfg := &sendConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Determine tools for this turn.
	tools := a.Tools
	if cfg.tools != nil {
		tools = cfg.tools
	}

	// Determine tool choice.
	toolChoice := resolveToolChoice(cfg)
	if cfg.toolChoice != nil && *cfg.toolChoice == None {
		tools = nil
		toolChoice = "none"
	}

	// Add the user message to conversation history.
	a.Messages = append(a.Messages, Message{Role: "user", Content: content})

	// Tool-use loop.
	for round := 0; round < maxToolRounds; round++ {
		req := &ChatRequest{
			Messages:   a.buildRequestMessages(),
			Tools:      tools,
			ToolChoice: toolChoice,
		}

		reply, err := a.Adapter.SendMessage(ctx, req)
		if err != nil {
			return Message{}, fmt.Errorf("adapter call failed: %w", err)
		}

		// No tool calls → final response.
		if len(reply.ToolCalls) == 0 {
			a.Messages = append(a.Messages, reply)
			return reply, nil
		}

		// Model wants to call tools — execute them.
		a.Messages = append(a.Messages, reply)

		for _, tc := range reply.ToolCalls {
			tool := findTool(tools, tc.Function.Name)
			if tool == nil {
				return Message{}, fmt.Errorf("model called unknown tool: %q", tc.Function.Name)
			}

			result, err := tool.Execute(ctx, json.RawMessage(tc.Function.Arguments))
			if err != nil {
				// Send the error to the model so it can recover.
				result = fmt.Sprintf("Error: %v", err)
			}

			a.Messages = append(a.Messages, Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}

		// After a forced tool call, switch to auto for subsequent rounds
		// to prevent infinite force loops.
		if cfg.forceTool != "" {
			toolChoice = "auto"
		}
	}

	return Message{}, fmt.Errorf("exceeded maximum tool rounds (%d)", maxToolRounds)
}

// buildRequestMessages prepends the system prompt (if set) to the
// conversation history.
func (a *Agent) buildRequestMessages() []Message {
	var msgs []Message
	if a.SystemPrompt != nil {
		msgs = append(msgs, Message{
			Role:    "system",
			Content: a.SystemPrompt(),
		})
	}
	msgs = append(msgs, a.Messages...)
	return msgs
}
