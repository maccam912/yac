package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/maccam912/yac"
	"github.com/maccam912/yac/tools"
)

// mockAdapter returns pre-configured responses, safe for concurrent use.
type mockAdapter struct {
	mu        sync.Mutex
	responses []yac.Message
	requests  []*yac.ChatRequest
	callIndex int
}

func (m *mockAdapter) SendMessage(ctx context.Context, req *yac.ChatRequest) (yac.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requests = append(m.requests, req)
	if m.callIndex >= len(m.responses) {
		return yac.Message{}, fmt.Errorf("no more mock responses")
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

func TestDelegateBasic(t *testing.T) {
	// Create a mock adapter that returns a simple text response.
	adapter := &mockAdapter{
		responses: []yac.Message{
			{Role: "assistant", Content: "The price is $28,000."},
		},
	}

	tool := tools.Delegate(tools.DelegateConfig{
		Adapter:  adapter,
		MaxDepth: 1,
	})

	if tool.Name != "delegate" {
		t.Fatalf("expected tool name 'delegate', got %q", tool.Name)
	}

	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{
			{"description": "Look up the price of a new Honda Civic"},
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "$28,000") {
		t.Errorf("expected result to contain price, got: %s", result)
	}
	if !strings.Contains(result, "Task 1") {
		t.Errorf("expected result to contain task header, got: %s", result)
	}
}

func TestDelegateParallelTasks(t *testing.T) {
	// We need separate adapters for each subagent since they run
	// concurrently and each gets its own agent.
	// Use a single thread-safe adapter that returns multiple responses.
	adapter := &mockAdapter{
		responses: []yac.Message{
			{Role: "assistant", Content: "Result A"},
			{Role: "assistant", Content: "Result B"},
			{Role: "assistant", Content: "Result C"},
		},
	}

	tool := tools.Delegate(tools.DelegateConfig{
		Adapter:  adapter,
		MaxDepth: 1,
	})

	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{
			{"description": "Task A"},
			{"description": "Task B"},
			{"description": "Task C"},
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All three tasks should appear in the result.
	if !strings.Contains(result, "Task 1") || !strings.Contains(result, "Task 2") || !strings.Contains(result, "Task 3") {
		t.Errorf("expected all task headers, got: %s", result)
	}
}

func TestDelegateEmptyTasks(t *testing.T) {
	tool := tools.Delegate(tools.DelegateConfig{
		Adapter:  &mockAdapter{},
		MaxDepth: 1,
	})

	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{},
	})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for empty tasks")
	}
	if !strings.Contains(err.Error(), "at least one task") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDelegateInvalidArgs(t *testing.T) {
	tool := tools.Delegate(tools.DelegateConfig{
		Adapter:  &mockAdapter{},
		MaxDepth: 1,
	})

	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDelegateSubagentSystemPrompt(t *testing.T) {
	adapter := &mockAdapter{
		responses: []yac.Message{
			{Role: "assistant", Content: "Done."},
		},
	}

	tool := tools.Delegate(tools.DelegateConfig{
		Adapter:  adapter,
		MaxDepth: 1,
	})

	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{
			{"description": "Do something"},
		},
	})

	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The subagent should have received a system prompt.
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.requests) == 0 {
		t.Fatal("no requests recorded")
	}
	req := adapter.requests[0]
	if len(req.Messages) == 0 || req.Messages[0].Role != "system" {
		t.Fatal("expected system message in subagent request")
	}
	sys := req.Messages[0].Content
	if !strings.Contains(sys, "subagent") {
		t.Errorf("expected subagent instructions in system prompt, got: %s", sys)
	}
}

func TestDelegateDepthLimiting(t *testing.T) {
	adapter := &mockAdapter{
		responses: []yac.Message{
			{Role: "assistant", Content: "Done."},
		},
	}

	// MaxDepth 1 means subagents should NOT get the delegate tool.
	tool := tools.Delegate(tools.DelegateConfig{
		Adapter:  adapter,
		MaxDepth: 1,
	})

	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{
			{"description": "Do something"},
		},
	})

	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// At depth 1, subagents should have no tools (no delegate tool added).
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	req := adapter.requests[0]
	for _, tool := range req.Tools {
		if tool.Name == "delegate" {
			t.Error("subagent at depth 1 should not have delegate tool")
		}
	}
}

func TestDelegateDepthAllowsRecursion(t *testing.T) {
	adapter := &mockAdapter{
		responses: []yac.Message{
			{Role: "assistant", Content: "Done."},
		},
	}

	// MaxDepth 2 means subagents SHOULD get the delegate tool (with depth 1).
	tool := tools.Delegate(tools.DelegateConfig{
		Adapter:  adapter,
		MaxDepth: 2,
	})

	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{
			{"description": "Do something"},
		},
	})

	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// At depth 2, subagents should have the delegate tool.
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	req := adapter.requests[0]
	found := false
	for _, tool := range req.Tools {
		if tool.Name == "delegate" {
			found = true
		}
	}
	if !found {
		t.Error("subagent at depth 2 should have delegate tool")
	}
}

func TestDelegateWithExtraTools(t *testing.T) {
	adapter := &mockAdapter{
		responses: []yac.Message{
			{Role: "assistant", Content: "Calculated: 42"},
		},
	}

	calcTool := tools.Calculator()
	tool := tools.Delegate(tools.DelegateConfig{
		Adapter:  adapter,
		Tools:    []*yac.Tool{calcTool},
		MaxDepth: 1,
	})

	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{
			{"description": "Calculate 6 * 7"},
		},
	})

	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Subagent should have the calculator tool.
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	req := adapter.requests[0]
	found := false
	for _, tool := range req.Tools {
		if tool.Name == "calculator" {
			found = true
		}
	}
	if !found {
		t.Error("subagent should have calculator tool")
	}
}

func TestDelegateDefaultMaxDepth(t *testing.T) {
	adapter := &mockAdapter{
		responses: []yac.Message{
			{Role: "assistant", Content: "Done."},
		},
	}

	// MaxDepth 0 should default to 3, meaning subagents get delegate
	// tool (depth would be 2).
	tool := tools.Delegate(tools.DelegateConfig{
		Adapter: adapter,
	})

	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{
			{"description": "Do something"},
		},
	})

	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	req := adapter.requests[0]
	found := false
	for _, tool := range req.Tools {
		if tool.Name == "delegate" {
			found = true
		}
	}
	if !found {
		t.Error("default depth 3 should give subagents the delegate tool")
	}
}

func TestDelegateSubagentError(t *testing.T) {
	// Adapter that returns an error.
	adapter := &mockAdapter{
		responses: []yac.Message{}, // no responses → error
	}

	tool := tools.Delegate(tools.DelegateConfig{
		Adapter:  adapter,
		MaxDepth: 1,
	})

	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{
			{"description": "This will fail"},
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("delegate tool itself should not error, got: %v", err)
	}

	// The result should contain the error from the subagent.
	if !strings.Contains(result, "Error:") {
		t.Errorf("expected error in result, got: %s", result)
	}
}

func TestDelegateCustomSystemPrompt(t *testing.T) {
	adapter := &mockAdapter{
		responses: []yac.Message{
			{Role: "assistant", Content: "Done."},
		},
	}

	tool := tools.Delegate(tools.DelegateConfig{
		Adapter:      adapter,
		MaxDepth:     1,
		SystemPrompt: "You specialize in Go programming.",
	})

	args, _ := json.Marshal(map[string]any{
		"tasks": []map[string]any{
			{"description": "Write a hello world"},
		},
	})

	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	req := adapter.requests[0]
	sys := req.Messages[0].Content
	if !strings.Contains(sys, "Go programming") {
		t.Errorf("expected custom system prompt, got: %s", sys)
	}
}
