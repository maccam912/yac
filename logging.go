package yac

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
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

// --- Ring buffer for log introspection ---

// LogBuffer is a thread-safe ring buffer that stores recent log lines.
type LogBuffer struct {
	mu    sync.Mutex
	lines []string
	pos   int
	cap   int
	total int
}

// NewLogBuffer creates a ring buffer that holds up to capacity lines.
func NewLogBuffer(capacity int) *LogBuffer {
	if capacity <= 0 {
		capacity = 200
	}
	return &LogBuffer{
		lines: make([]string, capacity),
		cap:   capacity,
	}
}

func (lb *LogBuffer) add(line string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.lines[lb.pos] = line
	lb.pos = (lb.pos + 1) % lb.cap
	lb.total++
}

// Recent returns the most recent n lines (or fewer if not enough exist).
func (lb *LogBuffer) Recent(n int) []string {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	count := lb.total
	if count > lb.cap {
		count = lb.cap
	}
	if n > count {
		n = count
	}
	if n <= 0 {
		return nil
	}

	result := make([]string, n)
	start := (lb.pos - n + lb.cap) % lb.cap
	for i := 0; i < n; i++ {
		result[i] = lb.lines[(start+i)%lb.cap]
	}
	return result
}

// Write implements io.Writer so the buffer can be used with log.SetOutput.
// Each Write call is treated as one log line (trailing newline stripped).
func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	line := strings.TrimRight(string(p), "\n")
	if line != "" {
		lb.add(line)
	}
	return len(p), nil
}

// DefaultLogBuffer is the global log buffer used by yac's logging functions.
// It holds the 200 most recent log lines.
var DefaultLogBuffer = NewLogBuffer(200)

// LogWriter returns an io.Writer that writes to both stderr and the
// DefaultLogBuffer. Use it with log.SetOutput to capture standard log
// output for agent introspection.
func LogWriter() io.Writer {
	return io.MultiWriter(os.Stderr, DefaultLogBuffer)
}

// bufferPlain writes a plain-text (no ANSI) log line to the default buffer.
func bufferPlain(format string, args ...any) {
	line := fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	DefaultLogBuffer.add(line)
}

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
	bufferPlain("%s◆ agent.send \"%s\"", prefix, truncate(content, 80))
}

// logAdapterCall logs an adapter request.
func logAdapterCall(depth int, msgCount int, model string) {
	logMu.Lock()
	defer logMu.Unlock()
	prefix := indent(depth)
	fmt.Fprintf(os.Stderr, "%s  %s├─ adapter%s %s(%d msgs, model: %s)%s\n",
		prefix, colorDim, colorReset,
		colorDim, msgCount, model, colorReset)
	bufferPlain("%s  ├─ adapter (%d msgs, model: %s)", prefix, msgCount, model)
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
	bufferPlain("%s  ├─ tool:%s %s", prefix, toolName, truncate(args, 120))
}

// logToolResult logs the result returned by a tool.
func logToolResult(depth int, toolName string, result string) {
	logMu.Lock()
	defer logMu.Unlock()
	prefix := indent(depth)
	fmt.Fprintf(os.Stderr, "%s  %s├─%s %stool:%s ← %s%s\n",
		prefix, colorDim, colorReset,
		colorGreen, toolName, truncate(result, 120), colorReset)
	bufferPlain("%s  ├─ tool:%s <- %s", prefix, toolName, truncate(result, 120))
}

// logToolError logs an error from a tool execution.
func logToolError(depth int, toolName string, errMsg string) {
	logMu.Lock()
	defer logMu.Unlock()
	prefix := indent(depth)
	fmt.Fprintf(os.Stderr, "%s  %s├─%s %stool:%s ✗ %s%s\n",
		prefix, colorDim, colorReset,
		colorRed, toolName, truncate(errMsg, 120), colorReset)
	bufferPlain("%s  ├─ tool:%s ERROR %s", prefix, toolName, truncate(errMsg, 120))
}

// logReply logs the assistant's final text response.
func logReply(depth int, content string) {
	logMu.Lock()
	defer logMu.Unlock()
	prefix := indent(depth)
	fmt.Fprintf(os.Stderr, "%s  %s└─%s %sreply:%s %s\n",
		prefix, colorDim, colorReset,
		colorWhite, colorReset, truncate(content, 120))
	bufferPlain("%s  └─ reply: %s", prefix, truncate(content, 120))
}
