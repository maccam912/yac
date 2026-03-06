package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maccam912/yac"
	"github.com/maccam912/yac/provider/openai"
)

func TestProvider_Complete_SimpleResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q, want application/json", ct)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("authorization = %q, want 'Bearer test-key'", auth)
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello from mock!",
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &openai.Provider{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: server.URL,
	}

	resp, err := p.Complete(context.Background(), &yac.Request{
		Messages: []yac.Message{
			{Role: yac.User, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Message.Content != "Hello from mock!" {
		t.Errorf("content = %q, want %q", resp.Message.Content, "Hello from mock!")
	}
	if resp.Message.Role != yac.Assistant {
		t.Errorf("role = %q, want %q", resp.Message.Role, yac.Assistant)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("prompt tokens = %d, want 10", resp.Usage.PromptTokens)
	}
}

func TestProvider_Complete_WithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify tools were sent in the request
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		tools, ok := req["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Error("expected tools in request")
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{
							{
								"id":   "call_abc123",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": `{"city":"Austin"}`,
								},
							},
						},
					},
				},
			},
			"usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 15},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &openai.Provider{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: server.URL,
	}

	resp, err := p.Complete(context.Background(), &yac.Request{
		Messages: []yac.Message{{Role: yac.User, Content: "weather?"}},
		Tools: []yac.ToolDef{
			{
				Name:        "get_weather",
				Description: "Get weather for a city",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.ID != "call_abc123" {
		t.Errorf("tool call ID = %q, want %q", tc.ID, "call_abc123")
	}
	if tc.Name != "get_weather" {
		t.Errorf("tool call name = %q, want %q", tc.Name, "get_weather")
	}
}

func TestProvider_Complete_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer server.Close()

	p := &openai.Provider{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: server.URL,
	}

	_, err := p.Complete(context.Background(), &yac.Request{
		Messages: []yac.Message{{Role: yac.User, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 429 status")
	}
}
