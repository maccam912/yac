package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/maccam912/yac"
)

// SearXNG returns a tool that performs web searches via a SearXNG instance.
//
// Requires the SEARXNG_URL environment variable to be set to a valid
// SearXNG instance URL (e.g. "https://searxng.example.com").
//
// The tool will only be available if SEARXNG_URL is configured via the
// ShouldInclude check.
//
// Example:
//
//	os.Setenv("SEARXNG_URL", "https://searxng.example.com")
//	agent := yac.Agent{
//	    Tools: yac.FilterTools([]*yac.Tool{tools.SearXNG()}),
//	}
//	reply, _ := agent.Send(ctx, "Search for Go programming tutorials")
func SearXNG() *yac.Tool {
	return &yac.Tool{
		Name:        "searxng_search",
		Description: "Search the web using SearXNG. Returns search results including titles, URLs, and snippets. Use this to find current information, research topics, or discover resources online.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query, e.g. 'Go programming best practices'",
				},
				"max_results": map[string]any{
					"type":        "number",
					"description": "Maximum number of results to return. Defaults to 10",
				},
			},
			"required": []string{"query"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Query      string `json:"query"`
				MaxResults int    `json:"max_results"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			if params.Query == "" {
				return "", fmt.Errorf("query is required")
			}

			if params.MaxResults <= 0 {
				params.MaxResults = 10
			}

			searxngURL := os.Getenv("SEARXNG_URL")
			if searxngURL == "" {
				return "", fmt.Errorf("SEARXNG_URL environment variable not set")
			}

			// Build search URL
			searchURL := fmt.Sprintf("%s/search", strings.TrimSuffix(searxngURL, "/"))
			u, err := url.Parse(searchURL)
			if err != nil {
				return "", fmt.Errorf("invalid SEARXNG_URL: %w", err)
			}

			// Add query parameters
			q := u.Query()
			q.Set("q", params.Query)
			q.Set("format", "json")
			u.RawQuery = q.Encode()

			// Create request
			req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
			if err != nil {
				return "", fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("User-Agent", "yac-searxng/1.0")

			// Make request with 30 second timeout
			client := &http.Client{
				Timeout: 30 * time.Second,
			}

			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("search request failed: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				return "", fmt.Errorf("search failed with status %d: %s", resp.StatusCode, string(bodyBytes))
			}

			// Parse JSON response
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return "", fmt.Errorf("failed to read response: %w", err)
			}

			var searchResp struct {
				Query   string `json:"query"`
				Results []struct {
					Title   string `json:"title"`
					URL     string `json:"url"`
					Content string `json:"content"`
					Engine  string `json:"engine"`
				} `json:"results"`
			}

			if err := json.Unmarshal(bodyBytes, &searchResp); err != nil {
				return "", fmt.Errorf("failed to parse search results: %w", err)
			}

			// Format results
			var result strings.Builder
			fmt.Fprintf(&result, "Search results for: %s\n\n", params.Query)

			if len(searchResp.Results) == 0 {
				result.WriteString("No results found.")
				return result.String(), nil
			}

			// Limit to max_results
			limit := params.MaxResults
			if limit > len(searchResp.Results) {
				limit = len(searchResp.Results)
			}

			for i := 0; i < limit; i++ {
				r := searchResp.Results[i]
				fmt.Fprintf(&result, "%d. %s\n", i+1, r.Title)
				fmt.Fprintf(&result, "   URL: %s\n", r.URL)
				if r.Content != "" {
					// Truncate long snippets
					content := r.Content
					if len(content) > 200 {
						content = content[:200] + "..."
					}
					fmt.Fprintf(&result, "   %s\n", content)
				}
				result.WriteString("\n")
			}

			fmt.Fprintf(&result, "Showing %d of %d total results.", limit, len(searchResp.Results))

			return result.String(), nil
		},
		ShouldInclude: func() bool {
			return os.Getenv("SEARXNG_URL") != ""
		},
	}
}
