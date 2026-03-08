// Example: Reset conversation with memory preservation.
//
// Demonstrates using the reset_conversation tool to save important
// context as persistent memories before clearing conversation history.
// This is useful for long-running agents that need to periodically
// shed context while retaining key information.
//
// Usage:
//
//	go run ./examples/reset_conversation/
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

	// Use a temp directory so it cleans up after itself.
	memDir, err := os.MkdirTemp("", "yac-reset-example-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(memDir)

	memoryCfg := tools.MemoryConfig{Dir: memDir}

	// Build a dynamic system prompt that includes essential memories.
	buildSystemPrompt := func() string {
		base := "You are a helpful assistant. You can save important information " +
			"as memories and reset the conversation when it gets too long. " +
			"When asked to reset, use the reset_conversation tool and include " +
			"any important facts as memories. Keep answers brief."
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
		Tools: append(
			tools.MemoryTools(memoryCfg),
			tools.ResetConversation(tools.ResetConversationConfig{
				Agent:     nil, // set below after agent is initialized
				MemoryDir: memDir,
			}),
		),
	}

	// Patch the reset tool's agent pointer now that agent exists.
	// (Same pattern as ClearContext — the tool needs the agent pointer.)
	for _, t := range agent.Tools {
		if t.Name == "reset_conversation" {
			agent.Tools[len(agent.Tools)-1] = tools.ResetConversation(tools.ResetConversationConfig{
				Agent:     &agent,
				MemoryDir: memDir,
			})
			break
		}
	}

	ctx := context.Background()

	// Step 1: Have a conversation and establish some facts.
	fmt.Println("=== Step 1: Establish context ===")
	reply, err := agent.Send(ctx, "Hi! My name is Alex and I'm working on a Rust project called Nexus. "+
		"It's a distributed key-value store. Remember that.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 2: Add more context.
	fmt.Println("=== Step 2: Add more context ===")
	reply, err = agent.Send(ctx, "The project uses Raft for consensus and RocksDB for storage. "+
		"The main issue I'm debugging is a split-brain scenario during network partitions.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 3: Reset the conversation, saving key facts.
	fmt.Println("=== Step 3: Reset with memories ===")
	reply, err = agent.Send(ctx, "Let's reset the conversation. Save the important details about "+
		"me and my project as memories before clearing. Mark my name and project as essential.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Step 4: Show that conversation is fresh but memories persist.
	fmt.Println("=== Step 4: Post-reset conversation ===")
	fmt.Printf("Messages in history: %d\n", len(agent.Messages))
	reply, err = agent.Send(ctx, "What do you know about me and my project? "+
		"Search your memories to find out.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Assistant: %s\n\n", reply.Content)

	// Show stored memory files.
	fmt.Println("=== Stored memory files ===")
	entries, _ := os.ReadDir(memDir)
	if len(entries) == 0 {
		fmt.Println("(no files)")
	}
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(memDir, e.Name()))
		fmt.Printf("--- %s ---\n%s\n\n", e.Name(), strings.TrimSpace(string(data)))
	}

	// Show final conversation history.
	fmt.Println("=== Final Conversation History ===")
	for i, msg := range agent.Messages {
		switch msg.Role {
		case "tool":
			fmt.Printf("[%d] %s (call_id=%s): %.80s\n", i, msg.Role, msg.ToolCallID, msg.Content)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					fmt.Printf("[%d] %s: calls %s(%.60s)\n", i, msg.Role, tc.Function.Name, tc.Function.Arguments)
				}
			} else {
				fmt.Printf("[%d] %s: %s\n", i, msg.Role, msg.Content)
			}
		default:
			fmt.Printf("[%d] %s: %s\n", i, msg.Role, msg.Content)
		}
	}
}
