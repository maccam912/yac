// Example: Agent with tool use.
//
// Demonstrates defining a tool, giving it to an agent, and letting
// the model decide when to call it. Agent.Send handles the full
// tool-call loop automatically.
//
// Usage:
//
//	go run ./examples/tool_use/
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

	// Define a tool that returns the current time.
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
		SystemPrompt: yac.StaticPrompt("You are a helpful assistant. Use the available tools when needed."),
		Tools:        []*yac.Tool{timeTool},
	}

	reply, err := agent.Send(context.Background(), "What time is it right now?")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant: %s\n", reply.Content)

	// Show the full conversation history.
	fmt.Println("\n=== Conversation History ===")
	for i, msg := range agent.Messages {
		switch msg.Role {
		case "tool":
			fmt.Printf("[%d] %s (call_id=%s): %s\n", i, msg.Role, msg.ToolCallID, msg.Content)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				fmt.Printf("[%d] %s: [tool_calls: ", i, msg.Role)
				for _, tc := range msg.ToolCalls {
					fmt.Printf("%s(%s) ", tc.Function.Name, tc.Function.Arguments)
				}
				fmt.Println("]")
			} else {
				fmt.Printf("[%d] %s: %s\n", i, msg.Role, msg.Content)
			}
		default:
			fmt.Printf("[%d] %s: %s\n", i, msg.Role, msg.Content)
		}
	}
}
