package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/maccam912/yac"
)

func TestClearContext(t *testing.T) {
	agent := &yac.Agent{
		Messages: []yac.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
			{Role: "user", Content: "What is 2+2?"},
			{Role: "assistant", Content: "4"},
			{Role: "user", Content: "Thanks, now let's start fresh"},
		},
	}

	tool := ClearContext(agent)

	if tool.Name != "clear_context" {
		t.Fatalf("expected tool name 'clear_context', got %q", tool.Name)
	}

	result, err := tool.Execute(context.Background(), json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Context cleared. Conversation history has been reset." {
		t.Fatalf("unexpected result: %q", result)
	}

	if len(agent.Messages) != 1 {
		t.Fatalf("expected 1 message after clear, got %d", len(agent.Messages))
	}

	if agent.Messages[0].Role != "user" || agent.Messages[0].Content != "Thanks, now let's start fresh" {
		t.Fatalf("expected last user message preserved, got %+v", agent.Messages[0])
	}
}

func TestClearContextNoMessages(t *testing.T) {
	agent := &yac.Agent{}

	tool := ClearContext(agent)
	result, err := tool.Execute(context.Background(), json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Context cleared. Conversation history has been reset." {
		t.Fatalf("unexpected result: %q", result)
	}

	if agent.Messages != nil {
		t.Fatalf("expected nil messages, got %v", agent.Messages)
	}
}

func TestClearContextNoUserMessages(t *testing.T) {
	agent := &yac.Agent{
		Messages: []yac.Message{
			{Role: "assistant", Content: "I'm ready to help."},
		},
	}

	tool := ClearContext(agent)
	_, err := tool.Execute(context.Background(), json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agent.Messages != nil {
		t.Fatalf("expected nil messages, got %v", agent.Messages)
	}
}
