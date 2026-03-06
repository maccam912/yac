package yac_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/maccam912/yac"
)

type mockProvider struct {
	responses []*yac.Response
	calls     int
}

func (m *mockProvider) Complete(_ context.Context, _ *yac.Request) (*yac.Response, error) {
	if m.calls >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses")
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

func TestSimpleAgent_ReturnsFinalMessage(t *testing.T) {
	provider := &mockProvider{
		responses: []*yac.Response{
			{Message: yac.Message{Role: yac.Assistant, Content: "Hello!"}},
		},
	}

	run := &yac.Run{
		Provider: provider,
		System:   "You are terse.",
		Messages: []yac.Message{{Role: yac.User, Content: "Hi"}},
	}

	got, err := yac.SimpleAgent(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Hello!" {
		t.Errorf("got %q, want %q", got, "Hello!")
	}
}

func TestSimpleAgent_ExecutesToolCalls(t *testing.T) {
	provider := &mockProvider{
		responses: []*yac.Response{
			{
				Message: yac.Message{
					Role: yac.Assistant,
					ToolCalls: []yac.ToolCall{
						{ID: "call_1", Name: "add", Args: `{"a":3,"b":4}`},
					},
				},
			},
			{Message: yac.Message{Role: yac.Assistant, Content: "7"}},
		},
	}

	toolCalled := false
	run := &yac.Run{
		Provider: provider,
		Tools: []yac.ToolDef{{
			Name:       "add",
			Parameters: []byte(`{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"required":["a","b"]}`),
		}},
		HandleTool: func(_ context.Context, call yac.ToolCall) (*yac.ToolResult, error) {
			toolCalled = true
			if call.Name != "add" {
				t.Errorf("tool name = %q, want %q", call.Name, "add")
			}
			return &yac.ToolResult{Output: "7"}, nil
		},
		MaxTurns: 2,
	}

	got, err := yac.SimpleAgent(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !toolCalled {
		t.Error("expected tool handler to run")
	}
	if got != "7" {
		t.Errorf("got %q, want %q", got, "7")
	}
}

func TestToolLoopAgent_ToolThenFinal(t *testing.T) {
	provider := &mockProvider{
		responses: []*yac.Response{
			// First response: requests tool call
			{
				Message: yac.Message{
					Role: yac.Assistant,
					ToolCalls: []yac.ToolCall{
						{ID: "call_1", Name: "add", Args: `{"a":3,"b":4}`},
					},
				},
			},
			// Second response: final answer
			{
				Message: yac.Message{
					Role:    yac.Assistant,
					Content: "The answer is 7.",
				},
			},
		},
	}

	toolCalled := false
	run := &yac.Run{
		Provider: provider,
		Messages: []yac.Message{{Role: yac.User, Content: "What is 3+4?"}},
		Tools: []yac.ToolDef{{
			Name:       "add",
			Parameters: []byte(`{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"required":["a","b"]}`),
		}},
		HandleTool: func(_ context.Context, call yac.ToolCall) (*yac.ToolResult, error) {
			toolCalled = true
			if call.Name != "add" {
				t.Errorf("tool name = %q, want %q", call.Name, "add")
			}
			return &yac.ToolResult{Output: "7"}, nil
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
		Provider: provider,
		Messages: []yac.Message{{Role: yac.User, Content: "loop forever"}},
		Tools:    []yac.ToolDef{{Name: "noop", Parameters: []byte(`{"type":"object"}`)}},
		HandleTool: func(_ context.Context, _ yac.ToolCall) (*yac.ToolResult, error) {
			return &yac.ToolResult{Output: "ok"}, nil
		},
		MaxTurns: 3,
	}

	_, err := yac.ToolLoopAgent(context.Background(), run)
	if err == nil {
		t.Fatal("expected error for exceeding max turns")
	}
}

func TestToolLoopAgent_RequiresHandleToolWhenToolReturned(t *testing.T) {
	run := &yac.Run{
		Provider: &mockProvider{responses: []*yac.Response{{Message: yac.Message{Role: yac.Assistant, ToolCalls: []yac.ToolCall{{ID: "1", Name: "noop", Args: "{}"}}}}}},
		Tools:    []yac.ToolDef{{Name: "noop", Parameters: []byte(`{"type":"object"}`)}},
	}

	_, err := yac.ToolLoopAgent(context.Background(), run)
	if err == nil {
		t.Fatal("expected error when HandleTool is nil")
	}
}

func TestToolLoopAgent_TrimsContextMessages(t *testing.T) {
	provider := &mockProvider{responses: []*yac.Response{{Message: yac.Message{Role: yac.Assistant, Content: "ok"}}}}
	run := &yac.Run{
		Provider:           provider,
		System:             "sys",
		Messages:           []yac.Message{{Role: yac.User, Content: "1"}, {Role: yac.User, Content: "2"}, {Role: yac.User, Content: "3"}},
		MaxContextMessages: 3,
	}

	_, err := yac.ToolLoopAgent(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
