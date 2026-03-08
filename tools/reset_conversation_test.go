package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccam912/yac"
)

func TestResetConversationBasic(t *testing.T) {
	agent := &yac.Agent{
		Messages: []yac.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
			{Role: "user", Content: "Let's start over"},
		},
	}

	tool := ResetConversation(ResetConversationConfig{Agent: agent})

	if tool.Name != "reset_conversation" {
		t.Fatalf("expected name 'reset_conversation', got %q", tool.Name)
	}

	args, _ := json.Marshal(map[string]any{})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Conversation reset") {
		t.Errorf("unexpected result: %s", result)
	}

	if len(agent.Messages) != 1 {
		t.Fatalf("expected 1 message after reset, got %d", len(agent.Messages))
	}
	if agent.Messages[0].Content != "Let's start over" {
		t.Errorf("expected last user message preserved, got %q", agent.Messages[0].Content)
	}
}

func TestResetConversationWithMemories(t *testing.T) {
	dir := tempMemoryDir(t)
	agent := &yac.Agent{
		Messages: []yac.Message{
			{Role: "user", Content: "Tell me about Go"},
			{Role: "assistant", Content: "Go is a language by Google."},
			{Role: "user", Content: "OK, let's move on to something else"},
		},
	}

	tool := ResetConversation(ResetConversationConfig{
		Agent:     agent,
		MemoryDir: dir,
	})

	args, _ := json.Marshal(map[string]any{
		"memories": []map[string]any{
			{
				"title":   "User is learning Go",
				"tags":    []string{"go", "user-preference"},
				"content": "The user expressed interest in learning Go programming.",
			},
			{
				"title":     "Preferred communication style",
				"tags":      []string{"preference"},
				"content":   "User likes concise, direct answers.",
				"essential": true,
			},
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should report saved memories.
	if !strings.Contains(result, "Saved 2 memories") {
		t.Errorf("expected 2 saved memories in result: %s", result)
	}

	// Should include essential memory title.
	if !strings.Contains(result, "Preferred communication style") {
		t.Errorf("expected essential memory title in result: %s", result)
	}

	// Conversation should be cleared.
	if len(agent.Messages) != 1 {
		t.Fatalf("expected 1 message after reset, got %d", len(agent.Messages))
	}

	// Memory files should exist on disk.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read memory dir: %v", err)
	}
	mdCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			mdCount++
		}
	}
	if mdCount != 2 {
		t.Errorf("expected 2 memory files, got %d", mdCount)
	}
}

func TestResetConversationNoMemoryDir(t *testing.T) {
	agent := &yac.Agent{
		Messages: []yac.Message{
			{Role: "user", Content: "Save something"},
		},
	}

	tool := ResetConversation(ResetConversationConfig{Agent: agent})

	// Memories param is provided but no MemoryDir — should still clear without error.
	args, _ := json.Marshal(map[string]any{
		"memories": []map[string]any{
			{"title": "Test", "content": "Data"},
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Conversation reset") {
		t.Errorf("unexpected result: %s", result)
	}
	// No "Saved" line since no memory dir.
	if strings.Contains(result, "Saved") {
		t.Errorf("should not report saved memories without memory dir: %s", result)
	}
}

func TestResetConversationEmptyAgent(t *testing.T) {
	agent := &yac.Agent{}
	tool := ResetConversation(ResetConversationConfig{Agent: agent})

	args, _ := json.Marshal(map[string]any{})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Conversation reset") {
		t.Errorf("unexpected result: %s", result)
	}
	if agent.Messages != nil {
		t.Errorf("expected nil messages, got %v", agent.Messages)
	}
}

func TestResetConversationSkipsEmptyMemories(t *testing.T) {
	dir := tempMemoryDir(t)
	agent := &yac.Agent{
		Messages: []yac.Message{
			{Role: "user", Content: "Test"},
		},
	}

	tool := ResetConversation(ResetConversationConfig{
		Agent:     agent,
		MemoryDir: dir,
	})

	args, _ := json.Marshal(map[string]any{
		"memories": []map[string]any{
			{"title": "Valid", "content": "Some content", "tags": []string{"test"}},
			{"title": "", "content": "No title"},   // skipped
			{"title": "No content", "content": ""}, // skipped
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Saved 1 memories") {
		t.Errorf("expected 1 saved memory: %s", result)
	}
}

func TestResetConversationPreservesExistingEssentials(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir) // aaa111 is essential: "How to configure Ollama"

	agent := &yac.Agent{
		Messages: []yac.Message{
			{Role: "user", Content: "Reset please"},
		},
	}

	tool := ResetConversation(ResetConversationConfig{
		Agent:     agent,
		MemoryDir: dir,
	})

	args, _ := json.Marshal(map[string]any{})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "How to configure Ollama") {
		t.Errorf("expected existing essential memory in result: %s", result)
	}
}

func TestResetConversationMemoryFileContent(t *testing.T) {
	dir := tempMemoryDir(t)
	agent := &yac.Agent{
		Messages: []yac.Message{
			{Role: "user", Content: "Test"},
		},
	}

	tool := ResetConversation(ResetConversationConfig{
		Agent:     agent,
		MemoryDir: dir,
	})

	args, _ := json.Marshal(map[string]any{
		"memories": []map[string]any{
			{
				"title":     "Important fact",
				"tags":      []string{"fact", "test"},
				"content":   "The sky is blue.",
				"essential": true,
			},
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Extract saved ID from result.
	parts := strings.Split(result, "memories: ")
	if len(parts) < 2 {
		t.Fatalf("could not find saved ID in result: %s", result)
	}
	id := strings.TrimSuffix(strings.Split(parts[1], ".")[0], ".")

	// Read and verify file content.
	data, err := os.ReadFile(filepath.Join(dir, id+".md"))
	if err != nil {
		t.Fatalf("failed to read memory file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "title: Important fact") {
		t.Error("missing title in file")
	}
	if !strings.Contains(content, "tags: [fact, test]") {
		t.Error("missing tags in file")
	}
	if !strings.Contains(content, "essential: true") {
		t.Error("missing essential flag in file")
	}
	if !strings.Contains(content, "The sky is blue.") {
		t.Error("missing content in file")
	}
}
