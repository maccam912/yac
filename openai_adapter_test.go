//go:build integration

package yac

import (
	"context"
	"os"
	"testing"

	"github.com/joho/godotenv"
)

func init() {
	// Load .env from the project root for all tests in this package.
	_ = godotenv.Load()
}

func TestOpenAIAdapterSendMessage(t *testing.T) {
	baseURL := os.Getenv("YAC_BASE_URL")
	apiKey := os.Getenv("YAC_API_KEY")
	model := os.Getenv("YAC_MODEL")

	if baseURL == "" {
		t.Skip("YAC_BASE_URL not set, skipping integration test")
	}

	adapter := &OpenAIAdapter{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}

	reply, err := adapter.SendMessage(context.Background(), &ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Say hello in exactly three words."},
		},
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if reply.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", reply.Role)
	}

	if reply.Content == "" {
		t.Error("expected non-empty content in reply")
	}

	t.Logf("Reply: %s", reply.Content)
}
