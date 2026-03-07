package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/maccam912/yac"
)

// WebRequest returns a tool that makes HTTP requests (GET, POST, etc.).
//
// Supports custom headers, request body, different HTTP methods, and
// basic timeout configuration. Returns the response body as a string.
//
// Example:
//
//	agent := yac.Agent{
//	    Tools: []*yac.Tool{tools.WebRequest()},
//	}
//	reply, _ := agent.Send(ctx, "Fetch the content from https://example.com")
func WebRequest() *yac.Tool {
	return &yac.Tool{
		Name:        "web_request",
		Description: "Make HTTP requests to URLs. Supports GET, POST, PUT, DELETE, PATCH methods with optional headers and body. Returns the response body as text. Use this to fetch web pages, call APIs, or retrieve data from the internet.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to request, e.g. 'https://api.example.com/data'",
				},
				"method": map[string]any{
					"type":        "string",
					"description": "HTTP method to use. Defaults to 'GET'",
					"enum":        []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD"},
				},
				"headers": map[string]any{
					"type":        "object",
					"description": "Optional HTTP headers as key-value pairs, e.g. {'Content-Type': 'application/json', 'Authorization': 'Bearer token'}",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Optional request body for POST, PUT, PATCH requests",
				},
				"timeout_seconds": map[string]any{
					"type":        "number",
					"description": "Request timeout in seconds. Defaults to 30",
				},
			},
			"required": []string{"url"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				URL            string            `json:"url"`
				Method         string            `json:"method"`
				Headers        map[string]string `json:"headers"`
				Body           string            `json:"body"`
				TimeoutSeconds float64           `json:"timeout_seconds"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			if params.URL == "" {
				return "", fmt.Errorf("url is required")
			}

			// Default method to GET
			if params.Method == "" {
				params.Method = "GET"
			}
			params.Method = strings.ToUpper(params.Method)

			// Default timeout to 30 seconds
			timeout := 30 * time.Second
			if params.TimeoutSeconds > 0 {
				timeout = time.Duration(params.TimeoutSeconds * float64(time.Second))
			}

			// Create request
			var bodyReader io.Reader
			if params.Body != "" {
				bodyReader = bytes.NewBufferString(params.Body)
			}

			req, err := http.NewRequestWithContext(ctx, params.Method, params.URL, bodyReader)
			if err != nil {
				return "", fmt.Errorf("failed to create request: %w", err)
			}

			// Add headers
			for key, value := range params.Headers {
				req.Header.Set(key, value)
			}

			// Set default User-Agent if not provided
			if req.Header.Get("User-Agent") == "" {
				req.Header.Set("User-Agent", "yac-web-request/1.0")
			}

			// Create client with timeout
			client := &http.Client{
				Timeout: timeout,
			}

			// Make request
			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()

			// Read response body
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return "", fmt.Errorf("failed to read response: %w", err)
			}

			// Build response summary
			var result strings.Builder
			fmt.Fprintf(&result, "HTTP %d %s\n", resp.StatusCode, resp.Status)
			fmt.Fprintf(&result, "\nHeaders:\n")
			for key, values := range resp.Header {
				for _, value := range values {
					fmt.Fprintf(&result, "%s: %s\n", key, value)
				}
			}
			fmt.Fprintf(&result, "\nBody:\n%s", string(bodyBytes))

			return result.String(), nil
		},
	}
}
