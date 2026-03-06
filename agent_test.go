package yac_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/maccam912/yac"
)

// mockProvider is a Provider that returns canned responses.
type mockProvider struct {
	responses []*yac.Response
	calls     int
}

func (m *mockProvider) Complete(_ context.Context, req *yac.Request) (*yac.Response, error) {
	if m.calls >= len(m.responses) {
		return nil, fmt.Errorf("mock: no more responses (call %d)", m.calls)
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

func TestSimpleAgent(t *testing.T) {
	provider := &mockProvider{
		responses: []*yac.Response{
			{Message: yac.Message{Role: yac.Assistant, Content: "Hello!"}},
		},
	}

	run := &yac.Run{
		Provider: provider,
		System:   "You are helpful.",
		Messages: []yac.Message{
			{Role: yac.User, Content: "Hi"},
		},
	}

	result, err := yac.SimpleAgent(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello!" {
		t.Errorf("got %q, want %q", result, "Hello!")
	}
	if provider.calls != 1 {
		t.Errorf("provider called %d times, want 1", provider.calls)
	}
}

func TestToolLoopAgent_NoToolCalls(t *testing.T) {
	provider := &mockProvider{
		responses: []*yac.Response{
			{Message: yac.Message{Role: yac.Assistant, Content: "42"}},
		},
	}

	run := &yac.Run{
		Provider:   provider,
		Messages:   []yac.Message{{Role: yac.User, Content: "what is 6*7?"}},
		HandleTool: func(_ context.Context, _ yac.ToolCall) (string, error) { return "", nil },
	}

	result, err := yac.ToolLoopAgent(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "42" {
		t.Errorf("got %q, want %q", result, "42")
	}
}

func TestToolLoopAgent_WithToolCall(t *testing.T) {
	provider := &mockProvider{
		responses: []*yac.Response{
			// Turn 1: model requests a tool call
			{Message: yac.Message{
				Role: yac.Assistant,
				ToolCalls: []yac.ToolCall{
					{ID: "call_1", Name: "add", Args: `{"a":3,"b":4}`},
				},
			}},
			// Turn 2: model gives final answer
			{Message: yac.Message{Role: yac.Assistant, Content: "The answer is 7."}},
		},
	}

	toolCalled := false
	run := &yac.Run{
		Provider: provider,
		Messages: []yac.Message{{Role: yac.User, Content: "add 3+4"}},
		HandleTool: func(_ context.Context, call yac.ToolCall) (string, error) {
			toolCalled = true
			if call.Name != "add" {
				t.Errorf("tool name = %q, want %q", call.Name, "add")
			}
			return "7", nil
		},
	}

	result, err := yac.ToolLoopAgent(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !toolCalled {
		t.Error("HandleTool was never called")
	}
	if result != "The answer is 7." {
		t.Errorf("got %q, want %q", result, "The answer is 7.")
	}
	if provider.calls != 2 {
		t.Errorf("provider called %d times, want 2", provider.calls)
	}
}

func TestToolLoopAgent_ExceedsMaxTurns(t *testing.T) {
	// Provider always returns tool calls, never a final answer.
	infiniteToolCalls := make([]*yac.Response, 20)
	for i := range infiniteToolCalls {
		infiniteToolCalls[i] = &yac.Response{
			Message: yac.Message{
				Role:      yac.Assistant,
				ToolCalls: []yac.ToolCall{{ID: fmt.Sprintf("call_%d", i), Name: "noop", Args: "{}"}},
			},
		}
	}

	provider := &mockProvider{responses: infiniteToolCalls}

	run := &yac.Run{
		Provider:   provider,
		Messages:   []yac.Message{{Role: yac.User, Content: "loop forever"}},
		HandleTool: func(_ context.Context, _ yac.ToolCall) (string, error) { return "ok", nil },
		MaxTurns:   3,
	}

	_, err := yac.ToolLoopAgent(context.Background(), run)
	if err == nil {
		t.Fatal("expected error for exceeding max turns")
	}
}

func TestToolLoopAgent_RequiresHandleTool(t *testing.T) {
	run := &yac.Run{
		Provider: &mockProvider{},
		Messages: []yac.Message{{Role: yac.User, Content: "hi"}},
		// HandleTool is nil
	}

	_, err := yac.ToolLoopAgent(context.Background(), run)
	if err == nil {
		t.Fatal("expected error when HandleTool is nil")
	}
}
