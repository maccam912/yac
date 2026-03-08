package tools

import (
	"context"
	"encoding/json"

	"github.com/maccam912/yac"
)

// ClearContext returns a tool that clears the agent's conversation history,
// effectively starting a new conversation. When called, it removes all
// messages from the agent except the most recent user message.
//
// The agent pointer is required so the tool can modify the conversation
// history directly.
//
// Example:
//
//	agent := &yac.Agent{
//	    Adapter: adapter,
//	    Tools:   []*yac.Tool{tools.ClearContext(&agent)},
//	}
func ClearContext(agent *yac.Agent) *yac.Tool {
	return &yac.Tool{
		Name:        "clear_context",
		Description: "Clear the conversation history and start fresh. Use this when the conversation context is no longer relevant or when you want to reset. The most recent user message is preserved.",
		Parameters: yac.Schema{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			// Find the last user message.
			var lastUser *yac.Message
			for i := len(agent.Messages) - 1; i >= 0; i-- {
				if agent.Messages[i].Role == "user" {
					lastUser = &agent.Messages[i]
					break
				}
			}

			if lastUser != nil {
				agent.Messages = []yac.Message{*lastUser}
			} else {
				agent.Messages = nil
			}

			return "Context cleared. Conversation history has been reset.", nil
		},
	}
}
