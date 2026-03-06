// Command yac is the CLI entry point for the YAC agent runtime.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maccam912/yac"
	"github.com/maccam912/yac/provider/openai"
	"github.com/maccam912/yac/sandbox/docker"
)

const defaultRunTimeout = 45 * time.Second

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	model := os.Getenv("YAC_MODEL")
	baseURL := os.Getenv("YAC_BASE_URL")

	if model == "" {
		model = "gpt-4o-mini"
	}

	provider := &openai.Provider{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: baseURL,
	}

	sandbox := &docker.Sandbox{}

	// Define a "run_python" tool.
	pythonToolDef := yac.ToolDef{
		Name:        "run_python",
		Description: "Execute Python code and return the output.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"code": {
					"type": "string",
					"description": "Python code to execute"
				}
			},
			"required": ["code"]
		}`),
	}

	handleTool := func(ctx context.Context, call yac.ToolCall) (*yac.ToolResult, error) {
		switch call.Name {
		case "run_python":
			var args struct{ Code string }
			if err := json.Unmarshal([]byte(call.Args), &args); err != nil {
				return nil, fmt.Errorf("parse args: %w", err)
			}
			execResult, err := sandbox.Exec(ctx, args.Code)
			return &yac.ToolResult{
				Stdout:   execResult.Stdout,
				Stderr:   execResult.Stderr,
				ExitCode: execResult.ExitCode,
			}, err
		default:
			return nil, fmt.Errorf("unknown tool: %s", call.Name)
		}
	}

	// Read the user's prompt from command-line args.
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: yac <prompt>")
		os.Exit(1)
	}
	prompt := os.Args[1]

	run := &yac.Run{
		Provider: provider,
		System:   "You are a helpful assistant. You can run Python code using the run_python tool. Be concise.",
		Messages: []yac.Message{
			{Role: yac.User, Content: prompt},
		},
		Tools:              []yac.ToolDef{pythonToolDef},
		HandleTool:         handleTool,
		MaxTurns:           5,
		MaxContextMessages: 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(ctx, defaultRunTimeout)
	defer cancel()

	result, err := yac.ToolLoopAgent(ctx, run)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}
