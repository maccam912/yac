package tools

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestBash(t *testing.T) {
	// Save original env var and restore after test
	original := os.Getenv("YAC_ENABLE_BASH")
	defer func() {
		if original != "" {
			os.Setenv("YAC_ENABLE_BASH", original)
		} else {
			os.Unsetenv("YAC_ENABLE_BASH")
		}
	}()

	tool := Bash()

	t.Run("ShouldInclude when YAC_ENABLE_BASH is set", func(t *testing.T) {
		os.Setenv("YAC_ENABLE_BASH", "1")
		if !tool.ShouldInclude() {
			t.Error("ShouldInclude should return true when YAC_ENABLE_BASH is set")
		}
	})

	t.Run("ShouldInclude when YAC_ENABLE_BASH is not set", func(t *testing.T) {
		os.Unsetenv("YAC_ENABLE_BASH")
		if tool.ShouldInclude() {
			t.Error("ShouldInclude should return false when YAC_ENABLE_BASH is not set")
		}
	})

	// Set env var for remaining tests
	os.Setenv("YAC_ENABLE_BASH", "1")

	t.Run("Simple echo command", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"command": "echo hello"})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if !strings.Contains(result, "hello") {
			t.Errorf("expected 'hello' in output, got: %s", result)
		}
	})

	t.Run("Command with stderr", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"command": "echo out && echo err >&2"})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if !strings.Contains(result, "out") {
			t.Errorf("expected stdout in output, got: %s", result)
		}
		if !strings.Contains(result, "STDERR:") {
			t.Errorf("expected STDERR label in output, got: %s", result)
		}
		if !strings.Contains(result, "err") {
			t.Errorf("expected stderr content in output, got: %s", result)
		}
	})

	t.Run("Non-zero exit code", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"command": "echo failing && exit 1"})
		result, err := tool.Execute(context.Background(), args)
		// Non-zero exit with output returns output + exit error, no error
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if !strings.Contains(result, "failing") {
			t.Errorf("expected output from failed command, got: %s", result)
		}
		if !strings.Contains(result, "Exit error") {
			t.Errorf("expected exit error info, got: %s", result)
		}
	})

	t.Run("No output command", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"command": "true"})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if result != "(no output)" {
			t.Errorf("expected '(no output)', got: %s", result)
		}
	})

	t.Run("Missing command", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{})
		_, err := tool.Execute(context.Background(), args)
		if err == nil {
			t.Error("expected error for missing command")
		}
		if !strings.Contains(err.Error(), "command is required") {
			t.Errorf("expected 'command is required' error, got: %v", err)
		}
	})

	t.Run("YAC_ENABLE_BASH not set", func(t *testing.T) {
		os.Unsetenv("YAC_ENABLE_BASH")
		args, _ := json.Marshal(map[string]any{"command": "echo test"})
		_, err := tool.Execute(context.Background(), args)
		if err == nil {
			t.Error("expected error when YAC_ENABLE_BASH not set")
		}
		if !strings.Contains(err.Error(), "YAC_ENABLE_BASH") {
			t.Errorf("expected YAC_ENABLE_BASH error, got: %v", err)
		}
	})

	t.Run("Timeout is capped at 300", func(t *testing.T) {
		os.Setenv("YAC_ENABLE_BASH", "1")
		args, _ := json.Marshal(map[string]any{"command": "echo fast", "timeout": 500})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if !strings.Contains(result, "fast") {
			t.Errorf("expected 'fast' in output, got: %s", result)
		}
	})
}
