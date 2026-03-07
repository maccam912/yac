// Example: Context length management.
//
// Demonstrates using ContextLength and AggressiveTrim to keep
// conversations within a model's context window. This is essential
// for long-running agents that accumulate large message histories.
//
// Usage:
//
//	go run ./examples/context_management/
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/maccam912/yac"
)

func main() {
	_ = godotenv.Load()

	// A simple tool so we can see AggressiveTrim in action.
	timeTool := &yac.Tool{
		Name:        "get_current_time",
		Description: "Get the current date and time",
		Parameters: yac.Schema{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			return time.Now().Format("2006-01-02 15:04:05 (Monday)"), nil
		},
	}

	agent := yac.Agent{
		Adapter: &yac.OpenAIAdapter{
			APIKey:  os.Getenv("YAC_API_KEY"),
			BaseURL: os.Getenv("YAC_BASE_URL"),
			Model:   os.Getenv("YAC_MODEL"),
		},
		SystemPrompt: yac.StaticPrompt("You are a helpful assistant. Use tools when needed. Keep answers brief."),

		// ContextLength caps how many tokens can be sent per request.
		// If the conversation exceeds this, older messages are dropped.
		// Set this to your model's actual context window size.
		ContextLength: 4096,

		// AggressiveTrim strips completed tool-call clusters from
		// every request. Tool exchanges are bulky and rarely useful
		// once the model has processed them. This keeps the context
		// lean so more room is available for actual conversation.
		AggressiveTrim: true,

		Tools: []*yac.Tool{timeTool},
	}

	// Have a multi-turn conversation to show context management.
	questions := []string{
		"What time is it?",
		"What day of the week is that?",
		"Thanks! Can you tell me the time one more time?",
	}

	for _, q := range questions {
		fmt.Printf("You: %s\n", q)

		reply, err := agent.Send(context.Background(), q)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
		fmt.Printf("Assistant: %s\n\n", reply.Content)
	}

	// Show what's in the agent's full history.
	fmt.Println("=== Full Agent History (kept internally) ===")
	for i, msg := range agent.Messages {
		switch msg.Role {
		case "tool":
			fmt.Printf("  [%d] %s (call_id=%s): %s\n", i, msg.Role, msg.ToolCallID, msg.Content)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				fmt.Printf("  [%d] %s: [tool_calls: ", i, msg.Role)
				for _, tc := range msg.ToolCalls {
					fmt.Printf("%s(%s) ", tc.Function.Name, tc.Function.Arguments)
				}
				fmt.Println("]")
			} else {
				fmt.Printf("  [%d] %s: %s\n", i, msg.Role, msg.Content)
			}
		default:
			fmt.Printf("  [%d] %s: %s\n", i, msg.Role, msg.Content)
		}
	}

	// Show what AggressiveTrim actually sends (tool clusters stripped).
	trimmed := yac.StripToolClusters(agent.Messages)
	fmt.Println()
	fmt.Println("=== After AggressiveTrim (what gets sent to API) ===")
	for i, msg := range trimmed {
		fmt.Printf("  [%d] %s: %s\n", i, msg.Role, msg.Content)
	}

	// Show the savings.
	fmt.Println()
	fullTokens := yac.EstimateTokens(agent.Messages)
	trimmedTokens := yac.EstimateTokens(trimmed)
	fmt.Printf("Full history:    %d messages, ~%d tokens\n", len(agent.Messages), fullTokens)
	fmt.Printf("After trim:      %d messages, ~%d tokens\n", len(trimmed), trimmedTokens)
	fmt.Printf("Savings:         %d messages, ~%d tokens\n",
		len(agent.Messages)-len(trimmed), fullTokens-trimmedTokens)
}
