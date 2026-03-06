package yac

import (
	"context"
	"encoding/json"
)

// Provider is the interface for LLM backends.
// Implementations live in provider/ subpackages.
type Provider interface {
	Complete(ctx context.Context, req *Request) (*Response, error)
}

// Request is what gets sent to a Provider.
type Request struct {
	Messages []Message
	Tools    []ToolDef // optional; omit if the agent has no tools
	Config   *Config   // optional; nil means provider defaults
}

// Response is what comes back from a Provider.
type Response struct {
	Message Message // the assistant's reply (may include ToolCalls)
	Usage   Usage   // token counts
}

// ToolDef describes a tool the model can call.
type ToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema for the arguments
}

// Config holds optional generation parameters.
type Config struct {
	Temperature *float64
	MaxTokens   *int
}

// Usage reports token consumption for a single completion.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
}
