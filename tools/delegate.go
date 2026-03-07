package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/maccam912/yac"
)

// DelegateConfig configures the subagent delegation tool.
type DelegateConfig struct {
	// Adapter is the LLM backend that subagents will use.
	Adapter yac.Adapter

	// Tools is the set of tools available to subagents (in addition
	// to the delegate tool itself, which is added automatically when
	// depth permits).
	Tools []*yac.Tool

	// MaxDepth limits how many levels of subagent nesting are allowed.
	// A value of 3 (the default when zero) means: the parent can
	// delegate, those subagents can delegate, and their subagents can
	// delegate one more time — but that final level cannot delegate
	// further.
	MaxDepth int

	// SystemPrompt is an optional additional instruction prepended to
	// the subagent's system message. If empty, only the default
	// subagent instructions are used.
	SystemPrompt string
}

// Delegate returns a tool that lets an agent delegate work to one or
// more subagents running concurrently. Each subagent is an independent
// Agent with its own conversation, tools, and system prompt.
//
// Subagents run in parallel goroutines. The tool blocks until all
// subagents complete, then returns a combined result.
//
// Subagents can themselves delegate further (creating a tree of agents)
// up to MaxDepth levels deep. At the deepest level the delegate tool
// is not provided, preventing further recursion.
//
// Example:
//
//	agent := yac.Agent{
//	    Adapter: adapter,
//	    Tools: []*yac.Tool{
//	        tools.Delegate(tools.DelegateConfig{
//	            Adapter:  adapter,
//	            Tools:    []*yac.Tool{searchTool, emailTool},
//	            MaxDepth: 3,
//	        }),
//	    },
//	}
func Delegate(cfg DelegateConfig) *yac.Tool {
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = 3
	}
	return delegateTool(cfg, cfg.MaxDepth)
}

// delegateTool builds the delegate tool for a given remaining depth.
func delegateTool(cfg DelegateConfig, depth int) *yac.Tool {
	return &yac.Tool{
		Name: "delegate",
		Description: "Delegate tasks to subagents that run concurrently. " +
			"Each task gets its own agent with a fresh conversation. " +
			"Use this when you need to perform multiple independent " +
			"steps in parallel, or when a task benefits from focused, " +
			"isolated reasoning. Each subagent can use tools and will " +
			"return its final answer. Only the final response from each " +
			"subagent is returned — the full conversation is not preserved.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"tasks": map[string]any{
					"type":        "array",
					"description": "List of tasks to delegate. Each task runs as an independent subagent in parallel.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"description": map[string]any{
								"type":        "string",
								"description": "What the subagent should do. Be specific — include all context it needs since it has no access to the parent conversation.",
							},
						},
						"required": []string{"description"},
					},
				},
			},
			"required": []string{"tasks"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Tasks []struct {
					Description string `json:"description"`
				} `json:"tasks"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if len(params.Tasks) == 0 {
				return "", fmt.Errorf("at least one task is required")
			}

			// Build the tool set for subagents.
			subTools := make([]*yac.Tool, len(cfg.Tools))
			copy(subTools, cfg.Tools)

			// If we have remaining depth, give subagents their own
			// delegate tool so they can recurse further.
			if depth > 1 {
				subTools = append(subTools, delegateTool(cfg, depth-1))
			}

			// Build the system prompt for subagents.
			sysPrompt := buildSubagentPrompt(cfg.SystemPrompt, depth)

			type result struct {
				index int
				desc  string
				reply string
				err   error
			}

			results := make([]result, len(params.Tasks))
			var wg sync.WaitGroup

			for i, task := range params.Tasks {
				wg.Add(1)
				go func(i int, desc string) {
					defer wg.Done()

					agent := yac.Agent{
						Adapter:      cfg.Adapter,
						SystemPrompt: yac.StaticPrompt(sysPrompt),
						Tools:        subTools,
					}

					reply, err := agent.Send(ctx, desc)
					results[i] = result{
						index: i,
						desc:  desc,
						reply: reply.Content,
						err:   err,
					}
				}(i, task.Description)
			}

			wg.Wait()

			// Format results.
			var sb strings.Builder
			for i, r := range results {
				if i > 0 {
					sb.WriteString("\n\n---\n\n")
				}
				fmt.Fprintf(&sb, "## Task %d: %s\n\n", i+1, r.desc)
				if r.err != nil {
					fmt.Fprintf(&sb, "Error: %v", r.err)
				} else {
					sb.WriteString(r.reply)
				}
			}
			return sb.String(), nil
		},
	}
}

// buildSubagentPrompt constructs the system prompt for a subagent.
func buildSubagentPrompt(extra string, depth int) string {
	var sb strings.Builder
	sb.WriteString("You are a focused subagent responsible for completing a specific task. ")
	sb.WriteString("Prioritize accuracy and completeness in your response. ")
	sb.WriteString("Use the tools available to you as needed to accomplish your task.\n\n")
	sb.WriteString("Important guidelines:\n")
	sb.WriteString("- Return your final answer with all important details, since only your ")
	sb.WriteString("final response will be sent back to the parent agent — the full ")
	sb.WriteString("conversation history is NOT returned.\n")
	sb.WriteString("- Include any relevant data, numbers, names, or context that the ")
	sb.WriteString("parent agent might need.\n")
	sb.WriteString("- If you encounter an error or cannot complete the task, explain what ")
	sb.WriteString("went wrong clearly.\n")

	if depth > 1 {
		sb.WriteString("- You have access to a 'delegate' tool to spawn your own subagents ")
		sb.WriteString("if the task can be broken into independent subtasks. ")
		fmt.Fprintf(&sb, "You can delegate up to %d more level(s) deep.\n", depth-1)
	}

	if extra != "" {
		sb.WriteString("\n")
		sb.WriteString(extra)
		sb.WriteString("\n")
	}

	return sb.String()
}
