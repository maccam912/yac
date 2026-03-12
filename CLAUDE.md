# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

yac (Yet Another Claw) is an agentic framework in Go. It provides a minimal, explicit API for building LLM-powered agents with tool use, conversation history, and context management. It works with any OpenAI-compatible API (OpenAI, OpenRouter, Ollama, llama.cpp, vLLM).

## Commands

```bash
task test              # Unit tests (no network needed)
task test-integration  # Integration tests (needs LLM backend running)
task smoke             # Run all examples as smoke tests (needs LLM backend)
task test-all          # Integration + smoke
task build             # Compile everything
task fmt               # Format code
task vet               # Static analysis
```

Run a single test: `go test -v -run TestName ./...`

Run a single example: `go run ./examples/<name>/`

CI runs: `go build ./...`, `go vet ./...`, `go test -v -count=1 ./...` (unit tests only, no LLM backend).

## Environment

Create a `.env` file in the project root (gitignored). All examples load it via `godotenv.Load()`.

```
YAC_API_KEY=not-needed
YAC_BASE_URL=http://127.0.0.1:8081/v1
YAC_MODEL=default
SEARXNG_URL=https://searxng.example.com  # Optional - enables SearXNG search tool
VIKUNJA_URL=https://vikunja.example.com  # Optional - enables Vikunja task tools
VIKUNJA_API_KEY=tk_xxxx                  # Optional - Vikunja API token
```

## Architecture

Single Go module (`github.com/maccam912/yac`). External dependencies: `godotenv`, OpenTelemetry (`go.opentelemetry.io/otel`).

**Core types (root package `yac`):**
- `Agent` — the central struct. Holds an `Adapter`, `SystemPrompt`, `Tools`, `Messages` (conversation history), and context management settings (`ContextLength`, `AggressiveTrim`). The `Send(ctx, message, ...opts)` method runs the full tool-use loop (up to 10 rounds).
- `Adapter` — interface with one method: `SendMessage(ctx, *ChatRequest) (Message, error)`. Only implementation is `OpenAIAdapter`.
- `Tool` — Name, Description, Parameters (JSON Schema as `map[string]any`), an `Execute` function, and optional `ShouldInclude() bool` for conditional inclusion.
- `Message` — Role/Content/ToolCalls/ToolCallID. Roles: "system", "user", "assistant", "tool".
- System prompt helpers: `StaticPrompt()`, `TemplatePrompt()`, `CachedPrompt()` — all return `func() string`.
- Per-turn options: `WithTools()`, `WithToolChoice()`, `ForceToolUse()`.
- Context trimming: `trimMessages()` drops oldest messages to fit budget; `StripToolClusters()` removes completed tool exchanges.
- `FilterTools([]*Tool)` — filters tools based on their `ShouldInclude` check; tools without a check are always included.

**Standard tools (`tools/` subpackage):**
- `Calculator()` — math expression evaluator (recursive descent parser)
- `Delegate(DelegateConfig)` — spawns concurrent subagents via goroutines. Subagents get their own `Agent` with isolated conversation. Supports recursive nesting up to `MaxDepth`.
- `WebRequest()` — HTTP client tool (like curl) supporting GET, POST, PUT, DELETE, PATCH with custom headers and body
- `SearXNG()` — web search via SearXNG instance. Requires `SEARXNG_URL` env var; only included if set (via `ShouldInclude`)
- `VikunjaTools()` — task management via Vikunja API (list/get/create/update/delete). Requires `VIKUNJA_URL` and `VIKUNJA_API_KEY` env vars; only included if both are set. Lists show compact summaries (ID + title); full details require explicit get.
- `MemoryTools(MemoryConfig)` — memory CRUD (create/list/search/recall/edit/remove) plus `ConsolidateMemories` for finding and merging duplicate memories in bulk.
- `AgentTools(agent, memoryDir)` — standard agent-bound tools that need a live `*Agent`; currently provides `reset_conversation`.

**Observability:**
- `tracing.go` — `InitTracing(ctx, serviceName)` sets up OTLP exporter for OpenTelemetry. Spans are created automatically in `Agent.Send()`, `OpenAIAdapter.SendMessage()`, tool executions, and delegate subagents.
- `logging.go` — Always-on pretty stderr logging with ANSI colors and tree-style indentation. Nesting depth is propagated via `context.Context` using `DepthFromContext()`/`ContextWithDepth()`. Delegate subagents automatically log at increased depth.
- `ClearContext(agent)` — tool that resets conversation history, keeping only the last user message.
- `ResetConversation(ResetConversationConfig)` — saves important context as memories and then clears conversation history. Combines memory creation and context clearing into a single atomic operation. Requires agent pointer; memory dir is optional.

**Key design pattern:** `Agent.Send()` works on a local copy of `Messages` and only commits to `a.Messages` on success, preventing history pollution from mid-turn failures.

## Adding Examples

1. Create `examples/<name>/main.go` following the existing pattern (load `.env`, create adapter from env vars, create agent, call `Send`)
2. If the example can run as a smoke test (non-interactive, terminates), add it to the `smoke` task in `Taskfile.yml`
3. Long-running or interactive examples (like `telegram_bot`) should NOT be added to smoke tests

## Conventions

- Integration tests use build tag `//go:build integration`
- Tools return `(string, error)` — errors are sent to the model as text so it can recover
- `Schema` is `map[string]any` — use standard JSON Schema structure with `"type"`, `"properties"`, `"required"`
- Unknown tool calls from the model fail the entire `Send()` call (not recoverable)
