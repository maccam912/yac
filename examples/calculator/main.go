// Example: Agent with the standard library calculator tool.
//
// Shows how to use a tool from the tools stdlib — just call the
// constructor and add it to the agent. Agent.Send handles the
// tool-call loop automatically.
//
// Usage:
//
//	go run ./examples/calculator/
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/maccam912/yac"
	"github.com/maccam912/yac/tools"
)

func main() {
	_ = godotenv.Load()

	agent := yac.Agent{
		Adapter: &yac.OpenAIAdapter{
			APIKey:  os.Getenv("YAC_API_KEY"),
			BaseURL: os.Getenv("YAC_BASE_URL"),
			Model:   os.Getenv("YAC_MODEL"),
		},
		SystemPrompt: yac.StaticPrompt("You are a helpful assistant. Use the calculator tool to solve math problems accurately."),
		Tools:        []*yac.Tool{tools.Calculator()},
	}

	reply, err := agent.Send(context.Background(), "What is (123 + 456) * 7 - sqrt(144)?")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant: %s\n", reply.Content)

	// Show conversation to see the tool call in action.
	fmt.Println("\n=== Conversation History ===")
	for i, msg := range agent.Messages {
		switch msg.Role {
		case "tool":
			fmt.Printf("[%d] %s (call_id=%s): %s\n", i, msg.Role, msg.ToolCallID, msg.Content)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					fmt.Printf("[%d] %s: calls %s(%s)\n", i, msg.Role, tc.Function.Name, tc.Function.Arguments)
				}
			} else {
				fmt.Printf("[%d] %s: %s\n", i, msg.Role, msg.Content)
			}
		default:
			fmt.Printf("[%d] %s: %s\n", i, msg.Role, msg.Content)
		}
	}
}
