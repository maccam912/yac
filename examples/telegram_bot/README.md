# Telegram Bot Example

A Telegram bot powered by a yac agent with memory, delegation, and proactive
Telegram notification support.

Each chat gets its own agent with persistent conversation history, context
management, and aggressive trimming to stay within token limits.

## Setup

### 1. Create a Telegram Bot

1. Open Telegram and message [@BotFather](https://t.me/BotFather)
2. Send `/newbot` and follow the prompts
3. Copy the bot token (looks like `123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11`)

### 2. Configure Environment

```bash
cp examples/telegram_bot/.env.example .env
```

Edit `.env` and set your `TELEGRAM_BOT_TOKEN` and LLM provider credentials.

The `.env.example` is pre-configured for **OpenRouter with gpt-oss-120b**:

```env
TELEGRAM_BOT_TOKEN=your-bot-token
YAC_API_KEY=sk-or-v1-your-openrouter-key
YAC_BASE_URL=https://openrouter.ai/api/v1
YAC_MODEL=gpt-oss-120b
```

### 3. Run

```bash
go run ./examples/telegram_bot/
```

## Usage

- Message your bot on Telegram with any question
- Try math: "What is 2^20 + sqrt(144)?"
- Try delegation: "Calculate these three things: 17*23, sqrt(256), and 2^16"
- Try reminders: `/remind 10m take a break`
- `/reset` — clear conversation history
- `/start` — welcome message
- `Ctrl+C` — stop the bot

## Features

- Per-chat conversation history
- Per-chat serialization so scheduled events and chat messages don't race
- Calculator tool for math expressions
- Delegate tool for parallel subtask execution
- Example-local `send_telegram_message` tool for proactive alerts
- `/remind <duration> <message>` to schedule a future wake-up event
- Context management (8192 token window, aggressive trimming)
- Graceful shutdown on Ctrl+C
- Long message splitting (Telegram's 4096 char limit)
- Reuses `TELEGRAM_BOT_TOKEN` from `.env`; the first chat that messages the bot becomes the default destination for proactive sends

## Proactive Alerts Pattern

The reminder flow demonstrates the pattern for "wake the bot later and let it
decide whether to notify the user":

1. The host app schedules an external event such as a timer.
2. When the event fires, the host calls `agent.Send()` with a system-event
   prompt instead of waiting for a user message.
3. The agent can choose to call `send_telegram_message` to notify the chat.

This keeps scheduling and event delivery in your application while leaving the
decision and message wording to the agent.
