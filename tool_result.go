package yac

// ToolResult is a structured output contract for tool handlers.
type ToolResult struct {
	Output   string            `json:"output,omitempty"`
	Error    string            `json:"error,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
	Stdout   string            `json:"stdout,omitempty"`
	Stderr   string            `json:"stderr,omitempty"`
	ExitCode int               `json:"exit_code,omitempty"`
}
