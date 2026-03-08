package yac

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIAdapterSendMessageFallbackCompletion(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"completion":"Here’s a quick rundown of the tools I can use:","reasoning":"internal text"}`)
	}))
	defer srv.Close()

	adapter := &OpenAIAdapter{APIKey: "test", BaseURL: srv.URL, Model: "test-model"}

	reply, err := adapter.SendMessage(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if reply.Role != "assistant" {
		t.Fatalf("expected role assistant, got %q", reply.Role)
	}
	if reply.Content != "Here’s a quick rundown of the tools I can use:" {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
}

func TestOpenAIAdapterSendMessageFallbackReasoning(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"completion":"","reasoning":"Fallback text"}`)
	}))
	defer srv.Close()

	adapter := &OpenAIAdapter{APIKey: "test", BaseURL: srv.URL, Model: "test-model"}

	reply, err := adapter.SendMessage(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if reply.Role != "assistant" {
		t.Fatalf("expected role assistant, got %q", reply.Role)
	}
	if reply.Content != "Fallback text" {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
}
