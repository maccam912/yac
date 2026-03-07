package yac

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// mockAdapter records requests and returns preconfigured responses.
type mockAdapter struct {
	responses []Message
	requests  []*ChatRequest
	callIndex int
}

func (m *mockAdapter) SendMessage(ctx context.Context, req *ChatRequest) (Message, error) {
	m.requests = append(m.requests, req)
	if m.callIndex >= len(m.responses) {
		return Message{}, fmt.Errorf("no more mock responses (call %d)", m.callIndex)
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

func TestSendBasic(t *testing.T) {
	mock := &mockAdapter{
		responses: []Message{{Role: "assistant", Content: "Hello!"}},
	}
	agent := Agent{Adapter: mock}

	reply, err := agent.Send(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if reply.Content != "Hello!" {
		t.Errorf("got %q, want %q", reply.Content, "Hello!")
	}
	if len(agent.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(agent.Messages))
	}
	if agent.Messages[0].Role != "user" || agent.Messages[1].Role != "assistant" {
		t.Error("unexpected message roles")
	}
}

func TestSendWithSystemPrompt(t *testing.T) {
	mock := &mockAdapter{
		responses: []Message{{Role: "assistant", Content: "I'm helpful!"}},
	}
	agent := Agent{
		Adapter:      mock,
		SystemPrompt: StaticPrompt("You are helpful."),
	}

	_, err := agent.Send(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	req := mock.requests[0]
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "You are helpful." {
		t.Error("system prompt not prepended correctly")
	}
}

func TestSendWithToolCall(t *testing.T) {
	greetTool := &Tool{
		Name:       "greet",
		Parameters: Schema{"type": "object"},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Name string `json:"name"`
			}
			json.Unmarshal(args, &p)
			return "Hello, " + p.Name + "!", nil
		},
	}

	mock := &mockAdapter{
		responses: []Message{
			// First: model calls tool.
			{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "greet", Arguments: `{"name":"Alice"}`}},
			}},
			// Second: model gives final answer.
			{Role: "assistant", Content: "I greeted Alice for you!"},
		},
	}

	agent := Agent{Adapter: mock, Tools: []*Tool{greetTool}}
	reply, err := agent.Send(context.Background(), "Say hi to Alice")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if reply.Content != "I greeted Alice for you!" {
		t.Errorf("got %q, want %q", reply.Content, "I greeted Alice for you!")
	}

	// History: user, assistant(tool_call), tool(result), assistant(final)
	if len(agent.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(agent.Messages))
	}
	if agent.Messages[2].Role != "tool" || agent.Messages[2].Content != "Hello, Alice!" {
		t.Errorf("tool result mismatch: %+v", agent.Messages[2])
	}
	if agent.Messages[2].ToolCallID != "call_1" {
		t.Errorf("tool_call_id mismatch: got %q", agent.Messages[2].ToolCallID)
	}
}

func TestSendWithToolOverride(t *testing.T) {
	defaultTool := &Tool{Name: "default_tool"}
	overrideTool := &Tool{Name: "override_tool"}

	mock := &mockAdapter{
		responses: []Message{{Role: "assistant", Content: "Done"}},
	}
	agent := Agent{Adapter: mock, Tools: []*Tool{defaultTool}}

	_, err := agent.Send(context.Background(), "Hello", WithTools(overrideTool))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	req := mock.requests[0]
	if len(req.Tools) != 1 || req.Tools[0].Name != "override_tool" {
		t.Errorf("expected override_tool, got %v", req.Tools)
	}
}

func TestSendWithToolChoiceNone(t *testing.T) {
	mock := &mockAdapter{
		responses: []Message{{Role: "assistant", Content: "No tools"}},
	}
	agent := Agent{Adapter: mock, Tools: []*Tool{{Name: "some_tool"}}}

	_, err := agent.Send(context.Background(), "Chat", WithToolChoice(None))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	req := mock.requests[0]
	if len(req.Tools) != 0 {
		t.Errorf("expected no tools, got %d", len(req.Tools))
	}
	if req.ToolChoice != "none" {
		t.Errorf("expected 'none', got %v", req.ToolChoice)
	}
}

func TestSendToolError(t *testing.T) {
	failTool := &Tool{
		Name:       "fail",
		Parameters: Schema{"type": "object"},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "", fmt.Errorf("something went wrong")
		},
	}

	mock := &mockAdapter{
		responses: []Message{
			{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "fail", Arguments: `{}`}},
			}},
			{Role: "assistant", Content: "Tool failed, sorry."},
		},
	}

	agent := Agent{Adapter: mock, Tools: []*Tool{failTool}}
	reply, err := agent.Send(context.Background(), "Do it")
	if err != nil {
		t.Fatalf("Send should not error (error goes to model): %v", err)
	}

	// Error should be sent to model as tool result.
	if agent.Messages[2].Content != "Error: something went wrong" {
		t.Errorf("expected error in tool result, got %q", agent.Messages[2].Content)
	}
	if reply.Content != "Tool failed, sorry." {
		t.Errorf("got %q", reply.Content)
	}
}

func TestSendUnknownTool(t *testing.T) {
	mock := &mockAdapter{
		responses: []Message{
			{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "nonexistent", Arguments: `{}`}},
			}},
		},
	}

	agent := Agent{Adapter: mock}
	_, err := agent.Send(context.Background(), "Do something")
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}
