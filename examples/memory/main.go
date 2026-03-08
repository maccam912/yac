// Example: Agent with memory tools for persistent knowledge storage.
//
// Demonstrates how the agent can create, search, recall, and remove
// memories stored as markdown files. Memories persist across sessions
// and are human-readable.
//
// Usage:
//
//	go run ./examples/memory/
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/maccam912/yac"
	"github.com/maccam912/yac/tools"
)

func main() {
	_ = godotenv.Load()

	// Use a temp directory for this example so it cleans up after itself.
	memDir, err := os.MkdirTemp("", "yac-memory-example-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(memDir)

	memoryCfg := tools.MemoryConfig{Dir: memDir}
	memoryTools := tools.MemoryTools(memoryCfg)

	// Build system prompt that includes any essential memories.
	buildSystemPrompt := func() string {
		base := "You are a helpful assistant with persistent memory. Use the memory tools to store and retrieve information."
		essentials := tools.EssentialMemories(memDir)
		if len(essentials) > 0 {
			base += "\n\nEssential memories:\n"
			for _, title := range essentials {
				base += "- " + title + "\n"
			}
		}
		return base
	}

	agent := yac.Agent{
		Adapter: &yac.OpenAIAdapter{
			APIKey:  os.Getenv("YAC_API_KEY"),
			BaseURL: os.Getenv("YAC_BASE_URL"),
			Model:   os.Getenv("YAC_MODEL"),
		},
		SystemPrompt: buildSystemPrompt,
		Tools:        memoryTools,
	}

	// Step 1: Ask the agent to create a memory.
	fmt.Println("=== Step 1: Create a memory ===")
	reply, err := agent.Send(context.Background(), "Please remember this: My preferred programming language is Go and I like to use the standard library whenever possible. Tag it with 'preferences' and 'go'. Mark it as essential since it's about my core preferences.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 2: Search for the memory.
	fmt.Println("=== Step 2: Search memories ===")
	reply, err = agent.Send(context.Background(), "Search your memories for anything tagged with 'preferences'.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 3: Show the stored files for transparency.
	fmt.Println("=== Stored memory files ===")
	entries, _ := os.ReadDir(memDir)
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(memDir, e.Name()))
		fmt.Printf("--- %s ---\n%s\n", e.Name(), strings.TrimSpace(string(data)))
	}

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
