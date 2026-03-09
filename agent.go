package yac

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// defaultMaxToolRounds is the default limit for tool-use round trips.
const defaultMaxToolRounds = 10

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

	// AggressiveTrim, when true, strips completed tool-call clusters
	// (assistant tool_calls + tool results) from the conversation
	// history on every request. This keeps the context lean since
	// tool exchanges are typically bulky and rarely useful once the
	// model has processed their results.
	AggressiveTrim bool

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

	// PostChatAction, if set, is called after each successful Send().
	// It returns a follow-up prompt that is sent to the model as a
	// second turn. The original reply is returned to the caller; the
	// follow-up exchange is only added to the conversation history.
	// This is useful for background tasks like memory management that
	// should happen after every user interaction.
	PostChatAction func() string

	// MaxToolRounds limits tool-use round trips per Send() call to
	// prevent infinite loops. Defaults to 10 if zero.
	MaxToolRounds int
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
	if a.Adapter == nil {
		return Message{}, fmt.Errorf("agent has no adapter configured")
	}

	depth := DepthFromContext(ctx)

	// Start an OTel span for this Send call.
	ctx, span := tracer.Start(ctx, "agent.send",
		trace.WithAttributes(
			attribute.String("yac.user_message", truncate(content, 200)),
			attribute.Int("yac.depth", depth),
		),
	)
	defer span.End()

	logSend(depth, content)

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

	span.SetAttributes(attribute.Int("yac.tool_count", len(tools)))

	// Work on a local copy of messages so that a.Messages is only
	// updated when the turn completes successfully.  This prevents
	// partial history pollution when an adapter call or tool lookup
	// fails mid-turn.
	pending := make([]Message, len(a.Messages), len(a.Messages)+2)
	copy(pending, a.Messages)

	// Add the user message to the local copy.
	pending = append(pending, Message{Role: "user", Content: content})

	// Pre-compute token budget used by tool definitions so we can
	// account for it when trimming conversation history.
	toolTokens := EstimateToolTokens(tools)

	// Tool-use loop.
	maxRounds := a.MaxToolRounds
	if maxRounds <= 0 {
		maxRounds = defaultMaxToolRounds
	}
	for round := 0; round < maxRounds; round++ {
		reqMessages := buildRequestMessages(a.SystemPrompt, pending)

		// Aggressive mode: strip completed tool-call clusters to
		// keep the context as lean as possible.
		if a.AggressiveTrim {
			reqMessages = StripToolClusters(reqMessages)
		}

		// If a context length is set, trim older messages so the
		// request stays within the model's context window.
		if a.ContextLength > 0 {
			reqMessages = trimMessages(reqMessages, a.ContextLength, toolTokens)
		}

		req := &ChatRequest{
			Messages:   reqMessages,
			Tools:      tools,
			ToolChoice: toolChoice,
		}

		logAdapterCall(depth, len(reqMessages), adapterModel(a.Adapter))

		reply, err := a.Adapter.SendMessage(ctx, req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return Message{}, fmt.Errorf("adapter call failed: %w", err)
		}

		// No tool calls → final response.
		if len(reply.ToolCalls) == 0 {
			pending = append(pending, reply)
			a.Messages = pending

			logReply(depth, reply.Content)
			span.SetAttributes(attribute.String("yac.reply", truncate(reply.Content, 200)))

			// Run post-chat action if configured. Temporarily nil
			// out PostChatAction to prevent recursive triggering.
			if a.PostChatAction != nil {
				action := a.PostChatAction
				a.PostChatAction = nil
				if prompt := action(); prompt != "" {
					_, _ = a.Send(ctx, prompt)
				}
				a.PostChatAction = action
			}

			return reply, nil
		}

		// Model wants to call tools — execute them.
		pending = append(pending, reply)

		for _, tc := range reply.ToolCalls {
			tool := findTool(tools, tc.Function.Name)
			if tool == nil {
				err := fmt.Errorf("model called unknown tool: %q", tc.Function.Name)
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				return Message{}, err
			}

			logToolCall(depth, tc.Function.Name, tc.Function.Arguments)

			// Create a child span for the tool execution.
			toolCtx, toolSpan := tracer.Start(ctx, "tool.execute",
				trace.WithAttributes(
					attribute.String("yac.tool.name", tc.Function.Name),
					attribute.String("yac.tool.args", truncate(tc.Function.Arguments, 500)),
					attribute.Int("yac.depth", depth),
				),
			)

			result, err := tool.Execute(toolCtx, json.RawMessage(tc.Function.Arguments))
			if err != nil {
				// Send the error to the model so it can recover.
				result = fmt.Sprintf("Error: %v", err)
				logToolError(depth, tc.Function.Name, err.Error())
				toolSpan.RecordError(err)
				toolSpan.SetStatus(codes.Error, err.Error())
			} else {
				logToolResult(depth, tc.Function.Name, result)
			}

			toolSpan.SetAttributes(attribute.String("yac.tool.result", truncate(result, 500)))
			toolSpan.End()

			pending = append(pending, Message{
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

	err := fmt.Errorf("exceeded maximum tool rounds (%d)", maxRounds)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return Message{}, err
}

// adapterModel extracts the model name from an adapter if it's an
// OpenAIAdapter. Returns "unknown" for other adapter types.
func adapterModel(a Adapter) string {
	if oa, ok := a.(*OpenAIAdapter); ok {
		return oa.Model
	}
	return "unknown"
}

// buildRequestMessages prepends the system prompt (if set) to the
// conversation history.
func buildRequestMessages(systemPrompt func() string, messages []Message) []Message {
	var msgs []Message
	if systemPrompt != nil {
		msgs = append(msgs, Message{
			Role:    "system",
			Content: systemPrompt(),
		})
	}
	msgs = append(msgs, messages...)
	return msgs
}
