package yac

import (
	"context"
	"errors"
	"fmt"
)

// AgentFunc is the signature for any agent. It receives a Run (its execution
// context) and returns a single string result. The parent sees only this
// string — no leaked reasoning, no bloated context.
type AgentFunc func(ctx context.Context, run *Run) (string, error)

// Run is the execution context handed to an AgentFunc.
type Run struct {
	Provider   Provider
	System     string    // system prompt
	Messages   []Message // seed conversation (the "context" for this agent)
	Tools      []ToolDef // tools this agent may call
	HandleTool func(ctx context.Context, call ToolCall) (string, error)
	MaxTurns   int // safety limit on LLM round-trips; 0 means default (10)
}

func (r *Run) maxTurns() int {
	if r.MaxTurns > 0 {
		return r.MaxTurns
	}
	return 10
}

// SimpleAgent calls the LLM once and returns its text response.
// It does not handle tool calls.
func SimpleAgent(ctx context.Context, run *Run) (string, error) {
	msgs := buildMessages(run.System, run.Messages)
	resp, err := run.Provider.Complete(ctx, &Request{
		Messages: msgs,
		Tools:    run.Tools,
		Config:   nil,
	})
	if err != nil {
		return "", fmt.Errorf("simple agent: %w", err)
	}
	return resp.Message.Content, nil
}

// ToolLoopAgent calls the LLM in a loop, handling tool calls until the model
// produces a final text response (no more tool calls).
func ToolLoopAgent(ctx context.Context, run *Run) (string, error) {
	if run.HandleTool == nil {
		return "", errors.New("tool loop agent: HandleTool is required")
	}

	msgs := buildMessages(run.System, run.Messages)
	max := run.maxTurns()

	for turn := 0; turn < max; turn++ {
		resp, err := run.Provider.Complete(ctx, &Request{
			Messages: msgs,
			Tools:    run.Tools,
		})
		if err != nil {
			return "", fmt.Errorf("tool loop agent turn %d: %w", turn, err)
		}

		// If no tool calls, we're done.
		if len(resp.Message.ToolCalls) == 0 {
			return resp.Message.Content, nil
		}

		// Append assistant message with tool calls.
		msgs = append(msgs, resp.Message)

		// Execute each tool call and append results.
		for _, tc := range resp.Message.ToolCalls {
			result, err := run.HandleTool(ctx, tc)
			if err != nil {
				result = fmt.Sprintf("error: %v", err)
			}
			msgs = append(msgs, Message{
				Role:    Tool,
				Content: result,
				ToolID:  tc.ID,
			})
		}
	}

	return "", fmt.Errorf("tool loop agent: exceeded %d turns", max)
}

// buildMessages prepends the system prompt to the conversation.
func buildMessages(system string, msgs []Message) []Message {
	out := make([]Message, 0, len(msgs)+1)
	if system != "" {
		out = append(out, Message{Role: System, Content: system})
	}
	out = append(out, msgs...)
	return out
}
