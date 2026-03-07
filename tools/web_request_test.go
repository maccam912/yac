package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebRequest(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo back request info
		switch r.URL.Path {
		case "/get":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("GET response"))
		case "/post":
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			body, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"received": "` + string(body) + `"}`))
		case "/headers":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Custom-Header: " + r.Header.Get("X-Test-Header")))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tool := WebRequest()

	tests := []struct {
		name      string
		args      map[string]any
		wantErr   bool
		wantMatch string // substring to match in response
	}{
		{
			name: "GET request",
			args: map[string]any{
				"url": server.URL + "/get",
			},
			wantMatch: "GET response",
		},
		{
			name: "POST request with body",
			args: map[string]any{
				"url":    server.URL + "/post",
				"method": "POST",
				"body":   "test data",
			},
			wantMatch: "test data",
		},
		{
			name: "Request with custom headers",
			args: map[string]any{
				"url": server.URL + "/headers",
				"headers": map[string]string{
					"X-Test-Header": "test-value",
				},
			},
			wantMatch: "test-value",
		},
		{
			name: "Missing URL",
			args: map[string]any{
				"method": "GET",
			},
			wantErr: true,
		},
		{
			name: "Invalid URL",
			args: map[string]any{
				"url": "not-a-valid-url",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argsJSON, _ := json.Marshal(tt.args)
			result, err := tool.Execute(context.Background(), argsJSON)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.wantMatch != "" && !strings.Contains(result, tt.wantMatch) {
				t.Errorf("expected result to contain %q, got: %s", tt.wantMatch, result)
			}
		})
	}
}

func TestWebRequestTimeout(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tool := WebRequest()

	args := map[string]any{
		"url":             server.URL,
		"timeout_seconds": 0.5, // 500ms timeout
	}

	argsJSON, _ := json.Marshal(args)
	_, err := tool.Execute(context.Background(), argsJSON)

	if err == nil {
		t.Error("expected timeout error but got none")
	}
	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "deadline") {
		t.Errorf("expected timeout/deadline error, got: %v", err)
	}
}
