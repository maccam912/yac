// Example: Agent with subagent delegation.
//
// Demonstrates using the delegate tool to spawn concurrent subagents
// that perform independent tasks in parallel and return their results.
//
// Usage:
//
//	go run ./examples/delegation/
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

	adapter := &yac.OpenAIAdapter{
		APIKey:  os.Getenv("YAC_API_KEY"),
		BaseURL: os.Getenv("YAC_BASE_URL"),
		Model:   os.Getenv("YAC_MODEL"),
	}

	// Create the delegate tool. Subagents share the same adapter and
	// can use the calculator. They can nest up to 3 levels deep.
	delegateTool := tools.Delegate(tools.DelegateConfig{
		Adapter:  adapter,
		Tools:    []*yac.Tool{tools.Calculator()},
		MaxDepth: 3,
	})

	agent := yac.Agent{
		Adapter: adapter,
		SystemPrompt: yac.StaticPrompt(
			"You are a helpful assistant that can delegate tasks to subagents. " +
				"When a user asks you to do multiple independent things, use the " +
				"delegate tool to run them in parallel. Each subagent works " +
				"independently and returns its result to you.",
		),
		Tools: []*yac.Tool{delegateTool},
	}

	reply, err := agent.Send(
		context.Background(),
		"I need three things done. Delegate to subagents to calculate these since the subagents have calculators: "+
			"1) Calculate 2^20, "+
			"2) Calculate the square root of 144, "+
			"3) Calculate (17 * 23) + (41 * 37)",
	)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant: %s\n", reply.Content)
}
