// Example: Agent with web request and search capabilities.
//
// This example demonstrates the web_request and searxng_search tools.
// The web_request tool makes HTTP requests (like curl), while the
// searxng_search tool searches the web via a SearXNG instance.
//
// Setup:
//
// Add to .env:
//
//	SEARXNG_URL=https://searxng.example.com  # Optional - enables search
//
// The searxng_search tool is only included if SEARXNG_URL is set, thanks
// to the tool's ShouldInclude check.
//
// Usage:
//
//	go run ./examples/web_tools/
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

	// Build the agent with web tools.
	// FilterTools will include searxng_search only if SEARXNG_URL is set.
	allTools := []*yac.Tool{
		tools.WebRequest(),
		tools.SearXNG(),
		tools.Calculator(), // Also include calculator for good measure
	}

	agent := yac.Agent{
		Adapter: &yac.OpenAIAdapter{
			APIKey:  os.Getenv("YAC_API_KEY"),
			BaseURL: os.Getenv("YAC_BASE_URL"),
			Model:   os.Getenv("YAC_MODEL"),
		},
		SystemPrompt: yac.StaticPrompt("You are a helpful assistant with access to the web. You can fetch web pages, search for information, and perform calculations."),
		Tools:        yac.FilterTools(allTools),
	}

	// Show which tools are enabled
	fmt.Println("=== Enabled Tools ===")
	for _, tool := range agent.Tools {
		fmt.Printf("- %s\n", tool.Name)
	}
	fmt.Println()

	// Example: Fetch a web page
	fmt.Println("=== Example 1: Fetching a web page ===")
	reply, err := agent.Send(context.Background(), "Fetch the content from https://httpbin.org/json and tell me what's in it.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Example: Web search (only if SearXNG is configured)
	if os.Getenv("SEARXNG_URL") != "" {
		fmt.Println("=== Example 2: Web search ===")
		reply, err = agent.Send(context.Background(), "Search for 'Go programming concurrency patterns' and summarize the top results.")
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
		fmt.Printf("Assistant: %s\n\n", reply.Content)
	} else {
		fmt.Println("=== Example 2: Web search ===")
		fmt.Println("Skipped (SEARXNG_URL not configured)")
		fmt.Println()
	}

	// Example: Combining tools
	fmt.Println("=== Example 3: Combining tools ===")
	reply, err = agent.Send(context.Background(), "Fetch https://httpbin.org/uuid twice and calculate the length difference of the returned UUIDs.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Show final conversation
	fmt.Println("=== Conversation Summary ===")
	for i, msg := range agent.Messages {
		switch msg.Role {
		case "user":
			fmt.Printf("[%d] User: %s\n", i, msg.Content)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					fmt.Printf("[%d] Assistant calls: %s\n", i, tc.Function.Name)
				}
			} else {
				// Truncate long responses
				content := msg.Content
				if len(content) > 100 {
					content = content[:100] + "..."
				}
				fmt.Printf("[%d] Assistant: %s\n", i, content)
			}
		case "tool":
			// Don't print tool results (too verbose)
			fmt.Printf("[%d] Tool result for call %s\n", i, msg.ToolCallID)
		}
	}
}
