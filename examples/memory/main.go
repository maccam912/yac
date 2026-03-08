// Example: Agent with memory tools for persistent knowledge storage.
//
// Demonstrates all memory tools: create, list, search, recall, edit,
// and remove. Memories are stored as markdown files and persist across
// sessions.
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
		base := "You are a helpful assistant with persistent memory. " +
			"Use the memory tools to store and retrieve information. " +
			"When asked to perform memory operations, always use the appropriate tool."
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

	ctx := context.Background()

	// Step 1: Create a couple of memories.
	fmt.Println("=== Step 1: Create memories ===")
	reply, err := agent.Send(ctx, "Please remember these two things as separate memories:\n"+
		"1. My preferred programming language is Go and I like to use the standard library whenever possible. Tag it with 'preferences' and 'go'. Mark it as essential.\n"+
		"2. My favorite editor is Neovim with a minimal config. Tag it with 'preferences' and 'editor'.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 2: List all memories.
	fmt.Println("=== Step 2: List all memories ===")
	reply, err = agent.Send(ctx, "List all of my memories.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 3: Search for memories by tag.
	fmt.Println("=== Step 3: Search memories by tag ===")
	reply, err = agent.Send(ctx, "Search your memories for anything tagged with 'preferences'.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 4: Recall a specific memory by ID.
	fmt.Println("=== Step 4: Recall a memory ===")
	reply, err = agent.Send(ctx, "Recall the full details of the Go preferences memory you just found.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 5: Edit a memory in place.
	fmt.Println("=== Step 5: Edit a memory ===")
	reply, err = agent.Send(ctx, "Update the editor memory: change the content to say I switched to Zed from Neovim, and update the title accordingly. Keep the existing tags.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 6: Verify the edit by recalling.
	fmt.Println("=== Step 6: Verify edit ===")
	reply, err = agent.Send(ctx, "Recall the editor memory to confirm the update went through.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 7: Remove a memory.
	fmt.Println("=== Step 7: Remove a memory ===")
	reply, err = agent.Send(ctx, "Remove the editor memory since I change editors too often to track.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 8: List again to confirm removal.
	fmt.Println("=== Step 8: List after removal ===")
	reply, err = agent.Send(ctx, "List all remaining memories to confirm the editor one is gone.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Show the stored files for transparency.
	fmt.Println("=== Stored memory files ===")
	entries, _ := os.ReadDir(memDir)
	if len(entries) == 0 {
		fmt.Println("(no files)")
	}
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(memDir, e.Name()))
		fmt.Printf("--- %s ---\n%s\n\n", e.Name(), strings.TrimSpace(string(data)))
	}

	fmt.Println("=== Conversation History ===")
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
