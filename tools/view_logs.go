package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/maccam912/yac"
)

// ViewLogs returns a tool that lets the agent read recent log output.
// This includes yac's internal logging (agent sends, tool calls, replies)
// and any standard log output routed through yac.LogWriter().
func ViewLogs() *yac.Tool {
	return &yac.Tool{
		Name:        "view_logs",
		Description: "View recent log lines from the application. Useful for debugging, checking reminder poller status, inspecting errors, or understanding what happened recently. Returns up to 100 most recent log lines by default.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"lines": map[string]any{
					"type":        "integer",
					"description": "Number of recent log lines to return (1-200). Defaults to 100.",
				},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Lines int `json:"lines"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if params.Lines <= 0 {
				params.Lines = 100
			}
			if params.Lines > 200 {
				params.Lines = 200
			}

			recent := yac.DefaultLogBuffer.Recent(params.Lines)
			if len(recent) == 0 {
				return "No log lines available.", nil
			}

			return strings.Join(recent, "\n"), nil
		},
	}
}
