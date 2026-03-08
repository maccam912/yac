package tools

import (
	"testing"

	"github.com/maccam912/yac"
)

func TestAll(t *testing.T) {
	tools := All()

	if len(tools) != 9 {
		t.Errorf("expected 9 tools, got %d", len(tools))
	}

	// Check that all expected tools are present
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	expected := []string{"calculator", "web_request", "searxng_search", "shell",
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

	if len(tools) != 10 {
		t.Errorf("expected 10 tools (9 base + delegate), got %d", len(tools))
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
