package yac

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

// --- Context key for agent nesting depth ---

type depthKey struct{}

// DepthFromContext returns the agent nesting depth from the context.
// Returns 0 for top-level agents.
func DepthFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(depthKey{}).(int); ok {
		return v
	}
	return 0
}

// ContextWithDepth returns a new context with the given nesting depth.
func ContextWithDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, depthKey{}, depth)
}

// --- ANSI color codes ---

const (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorWhite  = "\033[1;37m"
	colorRed    = "\033[31m"
	colorDim    = "\033[2m"
)

// logMu serializes log output to prevent interleaved lines from
// concurrent subagents.
var logMu sync.Mutex

// indent builds the tree-style prefix for a given depth.
// depth=0: ""
// depth=1: "  │ "
// depth=2: "  │   │ "
func indent(depth int) string {
	if depth == 0 {
		return ""
	}
	var sb strings.Builder
	for i := 0; i < depth; i++ {
		sb.WriteString("  │ ")
	}
	return sb.String()
}

// truncate shortens s to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	// Replace newlines with ␊ for single-line display.
	s = strings.ReplaceAll(s, "\n", "␊")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// logSend logs a user message being sent to an agent.
func logSend(depth int, content string) {
	logMu.Lock()
	defer logMu.Unlock()
	prefix := indent(depth)
	fmt.Fprintf(os.Stderr, "%s%s◆ agent.send%s %s\"%s\"%s\n",
		prefix, colorCyan, colorReset,
		colorDim, truncate(content, 80), colorReset)
}

// logAdapterCall logs an adapter request.
func logAdapterCall(depth int, msgCount int, model string) {
	logMu.Lock()
	defer logMu.Unlock()
	prefix := indent(depth)
	fmt.Fprintf(os.Stderr, "%s  %s├─ adapter%s %s(%d msgs, model: %s)%s\n",
		prefix, colorDim, colorReset,
		colorDim, msgCount, model, colorReset)
}

// logToolCall logs when the model invokes a tool.
func logToolCall(depth int, toolName string, args string) {
	logMu.Lock()
	defer logMu.Unlock()
	prefix := indent(depth)
	fmt.Fprintf(os.Stderr, "%s  %s├─%s %stool:%s%s %s%s%s\n",
		prefix, colorDim, colorReset,
		colorYellow, toolName, colorReset,
		colorDim, truncate(args, 120), colorReset)
}

// logToolResult logs the result returned by a tool.
func logToolResult(depth int, toolName string, result string) {
	logMu.Lock()
	defer logMu.Unlock()
	prefix := indent(depth)
	fmt.Fprintf(os.Stderr, "%s  %s├─%s %stool:%s ← %s%s\n",
		prefix, colorDim, colorReset,
		colorGreen, toolName, truncate(result, 120), colorReset)
}

// logToolError logs an error from a tool execution.
func logToolError(depth int, toolName string, errMsg string) {
	logMu.Lock()
	defer logMu.Unlock()
	prefix := indent(depth)
	fmt.Fprintf(os.Stderr, "%s  %s├─%s %stool:%s ✗ %s%s\n",
		prefix, colorDim, colorReset,
		colorRed, toolName, truncate(errMsg, 120), colorReset)
}

// logReply logs the assistant's final text response.
func logReply(depth int, content string) {
	logMu.Lock()
	defer logMu.Unlock()
	prefix := indent(depth)
	fmt.Fprintf(os.Stderr, "%s  %s└─%s %sreply:%s %s\n",
		prefix, colorDim, colorReset,
		colorWhite, colorReset, truncate(content, 120))
}
