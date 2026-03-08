package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maccam912/yac"
)

// ResetConversationConfig configures the reset_conversation tool.
type ResetConversationConfig struct {
	// Agent is the agent whose conversation will be reset.
	Agent *yac.Agent

	// MemoryDir is the directory where memories are stored.
	// If empty, no memories are saved during reset.
	MemoryDir string
}

// ResetConversation returns a tool that saves important context as memories
// and then clears the conversation history, preserving only the last user message.
//
// This combines the functionality of create_memory and clear_context into a
// single atomic operation: the agent can checkpoint important information
// before starting fresh.
//
// Example:
//
//	agent := &yac.Agent{...}
//	tool := tools.ResetConversation(tools.ResetConversationConfig{
//	    Agent:     agent,
//	    MemoryDir: "./memories",
//	})
func ResetConversation(cfg ResetConversationConfig) *yac.Tool {
	return &yac.Tool{
		Name: "reset_conversation",
		Description: "Save important context as memories and then clear the conversation history to start fresh. " +
			"Use this when the conversation has grown long or shifted topics, and you want to preserve key " +
			"information before resetting. Each memory gets a title, tags, and content. " +
			"After reset, only the last user message is preserved, and you receive a summary of essential memories.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"memories": map[string]any{
					"type":        "array",
					"description": "Memories to save before clearing context. Omit or pass empty array to reset without saving.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"title": map[string]any{
								"type":        "string",
								"description": "A short summary of what this memory is about.",
							},
							"tags": map[string]any{
								"type":        "array",
								"description": "Tags for categorizing this memory.",
								"items":       map[string]any{"type": "string"},
							},
							"content": map[string]any{
								"type":        "string",
								"description": "The detailed content to remember.",
							},
							"essential": map[string]any{
								"type":        "boolean",
								"description": "If true, this memory's title is always visible in context. Use sparingly.",
							},
						},
						"required": []string{"title", "content"},
					},
				},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Memories []struct {
					Title     string   `json:"title"`
					Tags      []string `json:"tags"`
					Content   string   `json:"content"`
					Essential bool     `json:"essential"`
				} `json:"memories"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			var savedIDs []string

			// Save memories if a memory directory is configured and memories were provided.
			if cfg.MemoryDir != "" && len(params.Memories) > 0 {
				if err := os.MkdirAll(cfg.MemoryDir, 0755); err != nil {
					return "", fmt.Errorf("failed to create memory directory: %w", err)
				}

				for _, m := range params.Memories {
					if m.Title == "" || m.Content == "" {
						continue
					}

					id := randomID()
					meta := memoryMeta{
						ID:        id,
						Title:     m.Title,
						Tags:      m.Tags,
						Essential: m.Essential,
						Created:   time.Now().UTC().Format(time.RFC3339),
					}

					fileContent := renderMemoryFile(meta, m.Content)
					filePath := filepath.Join(cfg.MemoryDir, id+".md")
					if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
						return "", fmt.Errorf("failed to write memory %q: %w", m.Title, err)
					}
					savedIDs = append(savedIDs, id)
				}
			}

			// Clear conversation history, preserving last user message.
			var lastUser *yac.Message
			for i := len(cfg.Agent.Messages) - 1; i >= 0; i-- {
				if cfg.Agent.Messages[i].Role == "user" {
					lastUser = &cfg.Agent.Messages[i]
					break
				}
			}

			if lastUser != nil {
				cfg.Agent.Messages = []yac.Message{*lastUser}
			} else {
				cfg.Agent.Messages = nil
			}

			// Build response.
			var sb strings.Builder
			sb.WriteString("Conversation reset.")

			if len(savedIDs) > 0 {
				fmt.Fprintf(&sb, " Saved %d memories: %s.", len(savedIDs), strings.Join(savedIDs, ", "))
			}

			// Include essential memory titles for immediate context.
			if cfg.MemoryDir != "" {
				essentials := EssentialMemories(cfg.MemoryDir)
				if len(essentials) > 0 {
					sb.WriteString("\n\nEssential memories (always active):")
					for _, title := range essentials {
						fmt.Fprintf(&sb, "\n- %s", title)
					}
				}
			}

			return sb.String(), nil
		},
	}
}
