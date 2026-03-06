package yac

import "context"

// ExecResult is structured output returned by sandbox executions.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Sandbox runs code in an isolated environment and returns its output.
type Sandbox interface {
	Exec(ctx context.Context, code string) (*ExecResult, error)
	Close() error
}
