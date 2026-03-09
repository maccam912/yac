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
- `/reset` — clear conversation history
- `/start` — welcome message
- `Ctrl+C` — stop the bot

## Features

- Per-chat conversation history
- Per-chat serialization so scheduled events and chat messages don't race
- Calculator tool for math expressions
- Delegate tool for parallel subtask execution
- Example-local `send_telegram_message` tool for proactive alerts
- Context management (8192 token window, aggressive trimming)
- Graceful shutdown on Ctrl+C
- Long message splitting (Telegram's 4096 char limit)
- Reuses `TELEGRAM_BOT_TOKEN` from `.env`; the first chat that messages the bot becomes the default destination for proactive sends

