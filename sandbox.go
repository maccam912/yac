package yac

import "context"

// Sandbox runs code in an isolated environment and returns its output.
type Sandbox interface {
	Exec(ctx context.Context, code string) (string, error)
	Close() error
}
