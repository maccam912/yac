// Example: Agent with the standard library shell tool.
//
// Demonstrates using the shell tool to run shell commands. The tool
// is gated by the YAC_ENABLE_SHELL environment variable, which this
// example sets automatically.
//
// Usage:
//
//	go run ./examples/bash_tool/
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

	// Enable the shell tool for this example.
	os.Setenv("YAC_ENABLE_SHELL", "1")

	agent := yac.Agent{
		Adapter: &yac.OpenAIAdapter{
			APIKey:  os.Getenv("YAC_API_KEY"),
			BaseURL: os.Getenv("YAC_BASE_URL"),
			Model:   os.Getenv("YAC_MODEL"),
		},
		SystemPrompt: yac.StaticPrompt("You are a helpful assistant. Use the shell tool to run shell commands when needed."),
		Tools:        yac.FilterTools([]*yac.Tool{tools.Shell()}),
	}

	reply, err := agent.Send(context.Background(), "What operating system is this? Use uname to find out.")
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
