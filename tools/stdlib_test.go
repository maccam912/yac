package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/maccam912/yac"
)

func TestAgentTools(t *testing.T) {
	agent := &yac.Agent{
		Messages: []yac.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "start over"},
		},
	}

	tools := AgentTools(agent, "")
	if len(tools) != 1 {
		t.Fatalf("expected 1 agent tool, got %d", len(tools))
	}
	if tools[0].Name != "reset_conversation" {
		t.Fatalf("expected reset_conversation, got %q", tools[0].Name)
	}

	args, _ := json.Marshal(map[string]any{})
	result, err := tools[0].Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Conversation reset") {
		t.Fatalf("unexpected result: %s", result)
	}
	if len(agent.Messages) != 1 || agent.Messages[0].Content != "start over" {
		t.Fatalf("expected only last user message to remain, got %#v", agent.Messages)
	}
}

func TestAll(t *testing.T) {
	tools := All()

	if len(tools) != 10 {
		t.Errorf("expected 10 tools, got %d", len(tools))
	}

	// Check that all expected tools are present
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	expected := []string{"calculator", "web_request", "searxng_search", "shell", "view_logs",
		"list_vikunja_tasks", "get_vikunja_task", "create_vikunja_task", "update_vikunja_task", "delete_vikunja_task"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected tool %q to be present", name)
		}
	}
}

func TestAllWithDelegate(t *testing.T) {
	adapter := &yac.OpenAIAdapter{
		APIKey:  "test-key",
		BaseURL: "http://localhost:8080",
		Model:   "test-model",
	}

	tools := AllWithDelegate(adapter, 2)

	if len(tools) != 11 {
		t.Errorf("expected 11 tools (10 base + delegate), got %d", len(tools))
	}

	// Check that delegate is present
	var hasDelegate bool
	for _, tool := range tools {
		if tool.Name == "delegate" {
			hasDelegate = true
			break
		}
	}

	if !hasDelegate {
		t.Error("expected delegate tool to be present")
	}
}
