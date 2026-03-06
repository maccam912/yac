package yac

import (
	"context"
	"encoding/json"
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
	HandleTool func(ctx context.Context, call ToolCall) (*ToolResult, error)
	MaxTurns   int // safety limit on LLM round-trips; 0 means default (10)

	// MaxContextMessages trims conversation history before each provider call.
	// 0 means no trimming.
	MaxContextMessages int
}

func (r *Run) maxTurns() int {
	if r.MaxTurns > 0 {
		return r.MaxTurns
	}
	return 10
}

// SimpleAgent executes a bounded tool loop and returns the final text response.
func SimpleAgent(ctx context.Context, run *Run) (string, error) {
	copyRun := *run
	if copyRun.MaxTurns == 0 {
		copyRun.MaxTurns = 1
	}
	return ToolLoopAgent(ctx, &copyRun)
}

// ToolLoopAgent calls the LLM in a loop, handling tool calls until the model
// produces a final text response (no more tool calls).
func ToolLoopAgent(ctx context.Context, run *Run) (string, error) {
	msgs := buildMessages(run.System, run.Messages)
	max := run.maxTurns()
	toolDefs := indexToolsByName(run.Tools)

	for turn := 0; turn < max; turn++ {
		trimmed := trimMessages(msgs, run.MaxContextMessages)
		resp, err := run.Provider.Complete(ctx, &Request{
			Messages: trimmed,
			Tools:    run.Tools,
		})
		if err != nil {
			return "", fmt.Errorf("tool loop agent turn %d: %w", turn, err)
		}

		// If no tool calls, we're done.
		if len(resp.Message.ToolCalls) == 0 {
			return resp.Message.Content, nil
		}
		if run.HandleTool == nil {
			return "", errors.New("tool loop agent: HandleTool is required when model returns tool calls")
		}

		// Append assistant message with tool calls.
		msgs = append(msgs, resp.Message)

		// Execute each tool call and append results.
		for _, tc := range resp.Message.ToolCalls {
			if err := ValidateToolCall(tc, toolDefs[tc.Name]); err != nil {
				msgs = append(msgs, Message{
					Role:    Tool,
					Content: mustMarshalToolResult(&ToolResult{Error: fmt.Sprintf("invalid tool args: %v", err)}),
					ToolID:  tc.ID,
				})
				continue
			}

			result, err := run.HandleTool(ctx, tc)
			if err != nil {
				result = &ToolResult{Error: err.Error()}
			}
			msgs = append(msgs, Message{
				Role:    Tool,
				Content: mustMarshalToolResult(result),
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

func trimMessages(msgs []Message, max int) []Message {
	if max <= 0 || len(msgs) <= max {
		return msgs
	}

	start := len(msgs) - max
	if msgs[0].Role == System {
		start = len(msgs) - (max - 1)
		if start < 1 {
			start = 1
		}
		out := make([]Message, 0, max)
		out = append(out, msgs[0])
		out = append(out, msgs[start:]...)
		return out
	}

	return msgs[start:]
}

func mustMarshalToolResult(result *ToolResult) string {
	if result == nil {
		result = &ToolResult{}
	}
	b, err := json.Marshal(result)
	if err != nil {
		return `{"error":"failed to marshal tool result"}`
	}
	return string(b)
}

func indexToolsByName(tools []ToolDef) map[string]ToolDef {
	out := make(map[string]ToolDef, len(tools))
	for _, td := range tools {
		out[td.Name] = td
	}
	return out
}
