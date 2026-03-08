# yac — Yet Another Claw

[![CI](https://github.com/maccam912/yac/actions/workflows/ci.yml/badge.svg)](https://github.com/maccam912/yac/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/maccam912/yac.svg)](https://pkg.go.dev/github.com/maccam912/yac)

An agentic framework in Go. Simple, composable, and built for flexibility.

## Features

- **Simple agent abstraction** — an agent is just a struct with an adapter, a system prompt, tools, and conversation history
- **OpenAI-compatible adapter** — works with OpenAI, OpenRouter, Ollama, llama.cpp, vLLM, or anything that speaks the OpenAI chat completions API
- **Flexible system prompts** — static strings, Go templates with live data, or cached renders. Mix and match with composable helpers
- **Per-turn tool control** — define a default toolset, override it per-turn, force a specific tool, or disable tools entirely
- **Automatic tool-use loop** — `agent.Send()` handles the full cycle: model calls tool → execute → feed result → repeat until final answer
- **Standard library of tools** — ready-to-use tools like `Calculator()` that you can drop in with one line
- **Zero magic** — no code generation, no reflection, no global state. Everything is explicit

## Install

```bash
go get github.com/maccam912/yac
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/maccam912/yac"
)

func main() {
    agent := yac.Agent{
        Adapter: &yac.OpenAIAdapter{
            APIKey:  "your-api-key",
            BaseURL: "https://api.openai.com/v1",
            Model:   "gpt-4o",
        },
        SystemPrompt: yac.StaticPrompt("You are a helpful assistant."),
    }

    reply, err := agent.Send(context.Background(), "What is the capital of France?")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(reply.Content)
}
```

## System Prompts

Three composable helpers that all produce a `func() string`:

```go
// Static — constant string, never changes.
agent.SystemPrompt = yac.StaticPrompt("You are a helpful assistant.")

// Template — re-rendered on every Send() call with fresh data.
tmpl := template.Must(template.New("sys").Parse(
    "You are a {{.Role}}. Today is {{.Day}}.",
))
agent.SystemPrompt = yac.TemplatePrompt(tmpl, func() any {
    return map[string]string{
        "Role": "coding assistant",
        "Day":  time.Now().Weekday().String(),
    }
})

// Cached — render once, reuse forever. Wraps any other prompt.
agent.SystemPrompt = yac.CachedPrompt(yac.TemplatePrompt(tmpl, dataFn))
```

## Tools

### Defining a tool

A tool is a name, description, JSON Schema for parameters, and a Go function:

```go
weatherTool := &yac.Tool{
    Name:        "get_weather",
    Description: "Get the current weather for a location",
    Parameters: yac.Schema{
        "type": "object",
        "properties": map[string]any{
            "location": map[string]any{
                "type":        "string",
                "description": "City name, e.g. 'San Francisco, CA'",
            },
        },
        "required": []string{"location"},
    },
    Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
        var p struct{ Location string `json:"location"` }
        json.Unmarshal(args, &p)
        return fmt.Sprintf("72°F in %s", p.Location), nil
    },
}
```

### Using tools

```go
agent := yac.Agent{
    Adapter: adapter,
    Tools:   []*yac.Tool{weatherTool, searchTool},  // default toolset
}

// Uses default tools, model decides whether to call them.
reply, _ := agent.Send(ctx, "What's the weather in SF?")

// Override tools for this turn only.
reply, _ = agent.Send(ctx, "Calculate this", yac.WithTools(calcTool))

// Force a specific tool.
reply, _ = agent.Send(ctx, "Get weather", yac.ForceToolUse("get_weather"))

// Disable tools entirely for this turn.
reply, _ = agent.Send(ctx, "Just chat", yac.WithToolChoice(yac.None))

// Model must use a tool, but picks which one.
reply, _ = agent.Send(ctx, "Do something", yac.WithToolChoice(yac.Required))
```

### Standard library tools

Drop-in tools that require no configuration:

```go
import "github.com/maccam912/yac/tools"

agent := yac.Agent{
    Adapter: adapter,
    Tools:   []*yac.Tool{tools.Calculator()},
}

reply, _ := agent.Send(ctx, "What is sqrt(3^2 + 4^2)?")
// The model calls calculator("sqrt(3^2 + 4^2)") → "5", then responds.
```

**Available tools:**

| Tool | Description |
|------|-------------|
| `tools.Calculator()` | Evaluates math expressions: arithmetic, exponents, functions (`sqrt`, `sin`, `log`, etc.), constants (`pi`, `e`) |
| `tools.AgentTools(agent, memoryDir)` | Returns the standard tools that need a live agent instance, currently `reset_conversation` |

## Adapters

### OpenAI-compatible (works with many providers)

```go
// OpenAI
adapter := &yac.OpenAIAdapter{
    APIKey:  "sk-...",
    BaseURL: "https://api.openai.com/v1",
    Model:   "gpt-4o",
}

// OpenRouter
adapter := &yac.OpenAIAdapter{
    APIKey:  "sk-or-...",
    BaseURL: "https://openrouter.ai/api/v1",
    Model:   "anthropic/claude-3.5-sonnet",
}

// Local llama.cpp / llama-server
adapter := &yac.OpenAIAdapter{
    APIKey:  "not-needed",
    BaseURL: "http://localhost:8080/v1",
    Model:   "default",
}

// Ollama
adapter := &yac.OpenAIAdapter{
    APIKey:  "not-needed",
    BaseURL: "http://localhost:11434/v1",
    Model:   "llama3.1",
}
```

## Examples

Runnable examples live in [`examples/`](examples/):

| Example | What it shows |
|---------|--------------|
| [`static_prompt`](examples/static_prompt) | Simplest agent with a constant system prompt |
| [`dynamic_prompt`](examples/dynamic_prompt) | Template-based prompt with live date/time |
| [`cached_prompt`](examples/cached_prompt) | Same template, but rendered once and cached |
| [`tool_use`](examples/tool_use) | Custom tool definition + automatic tool-call loop |
| [`calculator`](examples/calculator) | Using a stdlib tool with one line |
| [`telegram_bot`](examples/telegram_bot) | Telegram bot with per-chat agents, memory, and scheduled wake-ups that can trigger proactive alerts |

Run any example:

```bash
go run ./examples/static_prompt/
```

## Development

This project uses [Task](https://taskfile.dev) as a cross-platform task runner:

```bash
task test              # unit tests only
task test-integration  # integration tests (needs LLM backend)
task smoke             # run all examples as smoke tests
task test-all          # integration + smoke
task build             # compile
task fmt               # format code
task vet               # static analysis
```

## License

MIT
