package yac

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"text/template"
)

func TestStaticPrompt(t *testing.T) {
	fn := StaticPrompt("You are a helpful assistant.")

	got := fn()
	if got != "You are a helpful assistant." {
		t.Errorf("got %q, want %q", got, "You are a helpful assistant.")
	}

	// Calling it again should return the same thing.
	if fn() != got {
		t.Error("StaticPrompt returned different values on consecutive calls")
	}
}

func TestStaticPromptEmpty(t *testing.T) {
	fn := StaticPrompt("")
	if fn() != "" {
		t.Errorf("expected empty string, got %q", fn())
	}
}

func TestTemplatePromptBasic(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse("Hello, {{.Name}}!"))

	fn := TemplatePrompt(tmpl, func() any {
		return map[string]string{"Name": "World"}
	})

	got := fn()
	if got != "Hello, World!" {
		t.Errorf("got %q, want %q", got, "Hello, World!")
	}
}

func TestTemplatePromptReRendersEachCall(t *testing.T) {
	var counter int
	tmpl := template.Must(template.New("test").Parse("Call #{{.N}}"))

	fn := TemplatePrompt(tmpl, func() any {
		counter++
		return map[string]int{"N": counter}
	})

	first := fn()
	second := fn()

	if first != "Call #1" {
		t.Errorf("first call: got %q, want %q", first, "Call #1")
	}
	if second != "Call #2" {
		t.Errorf("second call: got %q, want %q", second, "Call #2")
	}
}

func TestTemplatePromptError(t *testing.T) {
	// Use missingkey=error so accessing a missing key triggers an error.
	tmpl := template.Must(template.New("test").Option("missingkey=error").Parse("{{.Missing}}"))

	fn := TemplatePrompt(tmpl, func() any {
		return map[string]string{}
	})

	got := fn()
	if !strings.HasPrefix(got, "[template error]") {
		t.Errorf("expected error prefix, got %q", got)
	}
}

func TestTemplatePromptMultiline(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse(
		`You are a {{.Role}}.
Available tools: {{range .Tools}}{{.}}, {{end}}
Focus: {{.Focus}}`))

	fn := TemplatePrompt(tmpl, func() any {
		return map[string]any{
			"Role":  "coding assistant",
			"Tools": []string{"search", "edit", "run"},
			"Focus": "refactoring",
		}
	})

	got := fn()
	if !strings.Contains(got, "coding assistant") {
		t.Errorf("expected 'coding assistant' in output, got %q", got)
	}
	if !strings.Contains(got, "search") {
		t.Errorf("expected 'search' in output, got %q", got)
	}
	if !strings.Contains(got, "refactoring") {
		t.Errorf("expected 'refactoring' in output, got %q", got)
	}
}

func TestCachedPromptCallsUnderlyingOnce(t *testing.T) {
	var callCount atomic.Int32

	inner := func() string {
		callCount.Add(1)
		return "cached value"
	}

	fn := CachedPrompt(inner)

	// Call multiple times.
	for i := 0; i < 10; i++ {
		got := fn()
		if got != "cached value" {
			t.Errorf("call %d: got %q, want %q", i, got, "cached value")
		}
	}

	if callCount.Load() != 1 {
		t.Errorf("inner function called %d times, want 1", callCount.Load())
	}
}

func TestCachedPromptThreadSafe(t *testing.T) {
	var callCount atomic.Int32

	inner := func() string {
		callCount.Add(1)
		return "thread-safe value"
	}

	fn := CachedPrompt(inner)

	// Hammer it from multiple goroutines.
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := fn()
			if got != "thread-safe value" {
				t.Errorf("got %q, want %q", got, "thread-safe value")
			}
		}()
	}
	wg.Wait()

	if callCount.Load() != 1 {
		t.Errorf("inner function called %d times under concurrency, want 1", callCount.Load())
	}
}

func TestCachedPromptWrapsTemplatePrompt(t *testing.T) {
	var renderCount atomic.Int32
	tmpl := template.Must(template.New("test").Parse("Render #{{.N}}"))

	// TemplatePrompt re-renders each call; CachedPrompt should freeze it.
	inner := TemplatePrompt(tmpl, func() any {
		renderCount.Add(1)
		return map[string]int32{"N": renderCount.Load()}
	})

	fn := CachedPrompt(inner)

	first := fn()
	second := fn()
	third := fn()

	if first != "Render #1" {
		t.Errorf("got %q, want %q", first, "Render #1")
	}
	if second != first || third != first {
		t.Error("CachedPrompt returned different values on subsequent calls")
	}
	if renderCount.Load() != 1 {
		t.Errorf("template rendered %d times, want 1", renderCount.Load())
	}
}

func TestAgentSystemPromptNil(t *testing.T) {
	agent := Agent{}

	// If SystemPrompt is nil, calling it would panic.
	// Users should check for nil before calling.
	if agent.SystemPrompt != nil {
		t.Error("expected nil SystemPrompt on zero-value Agent")
	}
}

func TestAgentWithStaticSystemPrompt(t *testing.T) {
	agent := Agent{
		SystemPrompt: StaticPrompt("You are a test agent."),
	}

	got := agent.SystemPrompt()
	if got != "You are a test agent." {
		t.Errorf("got %q, want %q", got, "You are a test agent.")
	}
}
