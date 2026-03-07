//go:build ignore
// +build ignore

package main

import (
	"fmt"
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

	// Build tools: all standard tools + delegate.
	allTools := tools.AllWithDelegate(adapter, 2)
	agentTools := yac.FilterTools(allTools)

	// Estimate tool tokens
	toolTokens := yac.EstimateToolTokens(agentTools)
	fmt.Printf("Tool tokens: %d\n", toolTokens)
	fmt.Printf("Number of tools: %d\n", len(agentTools))

	for _, tool := range agentTools {
		singleToolTokens := yac.EstimateToolTokens([]*yac.Tool{tool})
		fmt.Printf("  - %s: ~%d tokens\n", tool.Name, singleToolTokens)
	}

	// Check what would happen with a typical conversation
	agent := &yac.Agent{
		Adapter: adapter,
		SystemPrompt: yac.StaticPrompt(
			"You are a helpful Telegram bot assistant. You can perform calculations, " +
				"fetch web pages, search the web, and delegate independent tasks to run in parallel. " +
				"Keep your responses concise and well-formatted for a chat interface. " +
				"When a user asks multiple independent questions, use the delegate tool " +
				"to answer them in parallel.",
		),
		Tools:          agentTools,
		ContextLength:  8192,
		AggressiveTrim: true,
	}

	// Simulate a few messages
	agent.Messages = []yac.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi! How can I help you today?"},
		{Role: "user", Content: "What is 2+2?"},
		{Role: "assistant", Content: "2+2 equals 4."},
		{Role: "user", Content: "sweet ok. What is breaking news today in Iran."},
	}

	// See what would be sent
	sysPrompt := agent.SystemPrompt()
	systemMsg := yac.Message{Role: "system", Content: sysPrompt}
	allMessages := append([]yac.Message{systemMsg}, agent.Messages...)

	fmt.Printf("\nSystem prompt tokens: ~%d\n", yac.EstimateTokens([]yac.Message{systemMsg}))
	fmt.Printf("Conversation tokens: ~%d\n", yac.EstimateTokens(agent.Messages))
	fmt.Printf("Total with system: ~%d\n", yac.EstimateTokens(allMessages))
	fmt.Printf("Total budget: %d\n", agent.ContextLength)
	fmt.Printf("Remaining for messages: %d\n", agent.ContextLength-toolTokens)

	// Test trimming
	fmt.Printf("\nBefore trimming: %d messages\n", len(agent.Messages))
	for i, msg := range agent.Messages {
		fmt.Printf("  %d. [%s] %s\n", i, msg.Role, msg.Content[:min(50, len(msg.Content))])
	}

	// Check if trimming would be needed
	totalTokens := yac.EstimateTokens(allMessages) + toolTokens
	fmt.Printf("\nWould trimming be triggered? %v\n", totalTokens > agent.ContextLength)
	if totalTokens > agent.ContextLength {
		fmt.Printf("  Over budget by: %d tokens\n", totalTokens-agent.ContextLength)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
