package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/maccam912/yac"
)

// Bash returns a tool that executes bash commands.
//
// Requires the YAC_ENABLE_BASH environment variable to be set to any
// non-empty value. The tool will only be available if this variable is
// configured via the ShouldInclude check.
//
// Commands run with a default timeout of 60 seconds. A custom timeout
// can be specified per invocation (max 300 seconds).
//
// Example:
//
//	os.Setenv("YAC_ENABLE_BASH", "1")
//	agent := yac.Agent{
//	    Tools: yac.FilterTools([]*yac.Tool{tools.Bash()}),
//	}
//	reply, _ := agent.Send(ctx, "List the files in the current directory")
func Bash() *yac.Tool {
	return &yac.Tool{
		Name:        "bash",
		Description: "Execute a bash command and return its output (stdout and stderr). Use this to run shell commands, scripts, or system utilities. Commands run with a default timeout of 60 seconds.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The bash command to execute, e.g. 'ls -la' or 'echo hello'",
				},
				"timeout": map[string]any{
					"type":        "number",
					"description": "Timeout in seconds (default 60, max 300)",
				},
			},
			"required": []string{"command"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Command string  `json:"command"`
				Timeout float64 `json:"timeout"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			if params.Command == "" {
				return "", fmt.Errorf("command is required")
			}

			if os.Getenv("YAC_ENABLE_BASH") == "" {
				return "", fmt.Errorf("YAC_ENABLE_BASH environment variable not set")
			}

			timeout := 60 * time.Second
			if params.Timeout > 0 {
				if params.Timeout > 300 {
					params.Timeout = 300
				}
				timeout = time.Duration(params.Timeout * float64(time.Second))
			}

			cmdCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			cmd := exec.CommandContext(cmdCtx, "bash", "-c", params.Command)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			var result strings.Builder
			if stdout.Len() > 0 {
				result.WriteString(stdout.String())
			}
			if stderr.Len() > 0 {
				if result.Len() > 0 {
					result.WriteString("\n")
				}
				fmt.Fprintf(&result, "STDERR:\n%s", stderr.String())
			}

			if err != nil {
				if cmdCtx.Err() == context.DeadlineExceeded {
					return result.String(), fmt.Errorf("command timed out after %v", timeout)
				}
				if result.Len() > 0 {
					// Return output along with exit error info
					fmt.Fprintf(&result, "\nExit error: %s", err.Error())
					return result.String(), nil
				}
				return "", fmt.Errorf("command failed: %w", err)
			}

			if result.Len() == 0 {
				return "(no output)", nil
			}

			return result.String(), nil
		},
		ShouldInclude: func() bool {
			return os.Getenv("YAC_ENABLE_BASH") != ""
		},
	}
}
