package yac

import (
	"bytes"
	"fmt"
	"sync"
	"text/template"
)

// StaticPrompt returns a SystemPrompt function that always returns
// the same string. This is the simplest way to set a system prompt:
//
//	agent.SystemPrompt = StaticPrompt("You are a helpful assistant.")
func StaticPrompt(s string) func() string {
	return func() string { return s }
}

// TemplatePrompt returns a SystemPrompt function that re-renders a
// Go text/template on every call using fresh data from dataFn.
//
// The template is compiled once; only the data is re-evaluated each
// invocation. This is useful for system prompts that include dynamic
// information like the current time, available tools, or task state:
//
//	tmpl := template.Must(template.New("sys").Parse(
//	    "You are a {{.Role}}. The time is {{.Time}}.",
//	))
//	agent.SystemPrompt = TemplatePrompt(tmpl, func() any {
//	    return map[string]any{
//	        "Role": "coding assistant",
//	        "Time": time.Now().Format(time.RFC3339),
//	    }
//	})
//
// If the template fails to execute, the returned string contains the
// error prefixed with "[template error]" so it surfaces visibly rather
// than silently producing an empty prompt.
func TemplatePrompt(tmpl *template.Template, dataFn func() any) func() string {
	return func() string {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, dataFn()); err != nil {
			return fmt.Sprintf("[template error] %v", err)
		}
		return buf.String()
	}
}

// CachedPrompt wraps any SystemPrompt function so that the underlying
// function is called exactly once, and the result is reused for all
// subsequent calls. This is thread-safe.
//
// Use this when you want to render a template (or run expensive logic)
// once at startup and then reuse the result:
//
//	agent.SystemPrompt = CachedPrompt(TemplatePrompt(tmpl, dataFn))
//
// For a constant string, prefer StaticPrompt which is inherently cached.
func CachedPrompt(fn func() string) func() string {
	var once sync.Once
	var result string
	return func() string {
		once.Do(func() { result = fn() })
		return result
	}
}
