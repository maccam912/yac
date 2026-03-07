# AGENTS.md

Guidelines for contributors (human or AI) working on this project.

## Task Runner

This project uses [Task](https://taskfile.dev) instead of Make for cross-platform
compatibility. Install it with:

```
winget install Task.Task
```

Run `task --list` to see all available commands.

## Examples as Smoke Tests

Every example in `examples/` doubles as a smoke test. When you add a new example:

1. **Create the example** in `examples/<name>/main.go`
2. **Add it to the `smoke` task** in `Taskfile.yml` following the existing pattern:
   ```yaml
   - cmd: echo "=== Smoke{{":"}} <name> ==="
   - go run ./examples/<name>/
   - cmd: echo ""
   ```
3. **Verify it works** by running `task smoke` against a live backend

The smoke tests run against a real LLM endpoint configured in `.env`. They are
part of `task test-all` alongside the integration tests.

### Quick reference

| Command              | What runs                              |
|----------------------|----------------------------------------|
| `task test`          | Unit tests only (no network)           |
| `task test-integration` | Integration tests (needs LLM backend) |
| `task smoke`         | All examples as smoke tests            |
| `task test-all`      | Integration tests + smoke tests        |

## Environment

Copy `.env.example` (or create `.env`) with:

```
YAC_API_KEY=not-needed
YAC_BASE_URL=http://127.0.0.1:8081/v1
YAC_MODEL=default
```

Adjust for your backend. The API key can be any value for local servers
that don't require auth.
