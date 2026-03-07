package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSearXNG(t *testing.T) {
	// Create a mock SearXNG server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		query := r.URL.Query().Get("q")
		if query == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Return mock search results
		response := map[string]any{
			"query": query,
			"results": []map[string]any{
				{
					"title":   "Go Programming Language",
					"url":     "https://go.dev",
					"content": "The Go programming language is an open source project to make programmers more productive.",
					"engine":  "google",
				},
				{
					"title":   "A Tour of Go",
					"url":     "https://go.dev/tour",
					"content": "Learn Go with interactive examples.",
					"engine":  "google",
				},
				{
					"title":   "Go by Example",
					"url":     "https://gobyexample.com",
					"content": "Go by Example is a hands-on introduction to Go using annotated example programs.",
					"engine":  "duckduckgo",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Save original env var and restore after test
	originalURL := os.Getenv("SEARXNG_URL")
	defer func() {
		if originalURL != "" {
			os.Setenv("SEARXNG_URL", originalURL)
		} else {
			os.Unsetenv("SEARXNG_URL")
		}
	}()

	tool := SearXNG()

	t.Run("ShouldInclude when SEARXNG_URL is set", func(t *testing.T) {
		os.Setenv("SEARXNG_URL", server.URL)
		if !tool.ShouldInclude() {
			t.Error("ShouldInclude should return true when SEARXNG_URL is set")
		}
	})

	t.Run("ShouldInclude when SEARXNG_URL is not set", func(t *testing.T) {
		os.Unsetenv("SEARXNG_URL")
		if tool.ShouldInclude() {
			t.Error("ShouldInclude should return false when SEARXNG_URL is not set")
		}
	})

	// Set URL for remaining tests
	os.Setenv("SEARXNG_URL", server.URL)

	t.Run("Basic search", func(t *testing.T) {
		args := map[string]any{
			"query": "golang programming",
		}
		argsJSON, _ := json.Marshal(args)
		result, err := tool.Execute(context.Background(), argsJSON)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}

		// Check that results contain expected data
		if !strings.Contains(result, "Go Programming Language") {
			t.Error("expected result to contain 'Go Programming Language'")
		}
		if !strings.Contains(result, "https://go.dev") {
			t.Error("expected result to contain 'https://go.dev'")
		}
	})

	t.Run("Search with max_results", func(t *testing.T) {
		args := map[string]any{
			"query":       "golang",
			"max_results": 2,
		}
		argsJSON, _ := json.Marshal(args)
		result, err := tool.Execute(context.Background(), argsJSON)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}

		// Should show 2 of 3 results
		if !strings.Contains(result, "Showing 2 of 3 total results") {
			t.Errorf("expected to show 2 of 3 results, got: %s", result)
		}

		// Should include first two results
		if !strings.Contains(result, "Go Programming Language") {
			t.Error("expected first result")
		}
		if !strings.Contains(result, "A Tour of Go") {
			t.Error("expected second result")
		}
	})

	t.Run("Missing query", func(t *testing.T) {
		args := map[string]any{}
		argsJSON, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), argsJSON)

		if err == nil {
			t.Error("expected error for missing query")
		}
		if !strings.Contains(err.Error(), "query is required") {
			t.Errorf("expected 'query is required' error, got: %v", err)
		}
	})

	t.Run("SEARXNG_URL not set", func(t *testing.T) {
		os.Unsetenv("SEARXNG_URL")
		args := map[string]any{
			"query": "test",
		}
		argsJSON, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), argsJSON)

		if err == nil {
			t.Error("expected error when SEARXNG_URL not set")
		}
		if !strings.Contains(err.Error(), "SEARXNG_URL") {
			t.Errorf("expected SEARXNG_URL error, got: %v", err)
		}
	})
}
