// Example: Telegram bot powered by a yac agent.
//
// Starts a Telegram bot that listens for messages and responds using
// a yac agent with all standard tools (calculator, web request, search,
// delegation). Each chat gets its own agent with persistent conversation
// history.
//
// Setup:
//
//  1. Create a bot via @BotFather on Telegram and copy the token.
//  2. Copy .env.example to .env and fill in your values.
//  3. Run: go run ./examples/telegram_bot/
//  4. Message your bot on Telegram.
//  5. (Optional) Set SEARXNG_URL to enable web search.
//
// The .env.example is pre-configured for OpenRouter with gpt-oss-120b.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/joho/godotenv"
	"github.com/maccam912/yac"
	"github.com/maccam912/yac/tools"
)

// --- Telegram API types (minimal subset) ---

type tgUpdate struct {
	UpdateID int        `json:"update_id"`
	Message  *tgMessage `json:"message"`
}

type tgMessage struct {
	MessageID int    `json:"message_id"`
	Chat      tgChat `json:"chat"`
	Text      string `json:"text"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

type tgResponse struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

// --- Telegram helpers ---

func getUpdates(ctx context.Context, token string, offset int) ([]tgUpdate, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf(
		"https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30",
		token, offset,
	), nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tgResp tgResponse
	if err := json.Unmarshal(body, &tgResp); err != nil {
		return nil, err
	}
	if !tgResp.OK {
		return nil, fmt.Errorf("telegram API error: %s", string(body))
	}
	return tgResp.Result, nil
}

func sendMessage(token string, chatID int64, text string) error {
	// Telegram messages max out at 4096 chars. Split if needed.
	for len(text) > 0 {
		chunk := text
		if len(chunk) > 4096 {
			// Try to split at a newline boundary.
			cut := strings.LastIndex(chunk[:4096], "\n")
			if cut < 1 {
				cut = 4096
			}
			chunk = text[:cut]
			text = text[cut:]
		} else {
			text = ""
		}

		resp, err := http.PostForm(
			fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token),
			url.Values{
				"chat_id": {strconv.FormatInt(chatID, 10)},
				"text":    {chunk},
			},
		)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

// --- Per-chat agent management ---

type chatAgents struct {
	mu     sync.Mutex
	agents map[int64]*yac.Agent
	cfg    agentConfig
}

type agentConfig struct {
	adapter   *yac.OpenAIAdapter
	tools     []*yac.Tool
	memoryDir string
}

func newChatAgents(cfg agentConfig) *chatAgents {
	return &chatAgents{
		agents: make(map[int64]*yac.Agent),
		cfg:    cfg,
	}
}

func (ca *chatAgents) getOrCreate(chatID int64) *yac.Agent {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	if agent, ok := ca.agents[chatID]; ok {
		return agent
	}

	// Each chat gets its own memory directory for persistent storage.
	chatMemDir := filepath.Join(ca.cfg.memoryDir, fmt.Sprintf("chat_%d", chatID))
	memoryCfg := tools.MemoryConfig{Dir: chatMemDir}
	chatTools := make([]*yac.Tool, 0, len(ca.cfg.tools)+len(tools.MemoryTools(memoryCfg))+1)
	chatTools = append(chatTools, ca.cfg.tools...)
	chatTools = append(chatTools, tools.MemoryTools(memoryCfg)...)

	systemTemplate := template.Must(template.New("system").Parse("You are a helpful Telegram bot assistant. You can perform calculations, " +
		"fetch web pages, search the web, run shell commands, delegate independent tasks to run in parallel, " +
		"and remember things using your memory tools. " +
		"When the user asks to reset, start over, or clear the conversation while preserving important context, use the reset_conversation tool. " +
		"Keep your responses concise and well-formatted for a chat interface. " +
		"When a user asks multiple independent questions, use the delegate tool " +
		"to answer them in parallel. Today is {{.DayOfWeek}}, {{.DateTime}}" +
		"{{if .EssentialMemories}}\n\nEssential memories:\n{{.EssentialMemories}}{{end}}"))

	agent := &yac.Agent{
		Adapter: ca.cfg.adapter,
		SystemPrompt: yac.TemplatePrompt(systemTemplate, func() any {
			now := time.Now()
			essentials := tools.EssentialMemories(chatMemDir)
			var essentialStr string
			for _, title := range essentials {
				essentialStr += "- " + title + "\n"
			}
			return map[string]string{
				"DateTime":          now.Format("2006-01-02 15:04:05"),
				"DayOfWeek":         now.Weekday().String(),
				"EssentialMemories": essentialStr,
			}
		}),
		Tools:          chatTools,
		ContextLength:  8192,
		AggressiveTrim: true,
		PostChatAction: yac.StaticPrompt(
			"[SYSTEM] Review the conversation above. If the user shared any new facts, preferences, " +
				"or information worth remembering, save or update memories now using your memory tools. " +
				"If nothing noteworthy was said, do nothing. Do not respond to the user — this is an " +
				"internal housekeeping step.",
		),
	}
	agent.Tools = append(chatTools, tools.AgentTools(agent, chatMemDir)...)
	ca.agents[chatID] = agent
	return agent
}

func main() {
	_ = godotenv.Load()

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required. Copy .env.example to .env and fill it in.")
	}

	adapter := &yac.OpenAIAdapter{
		APIKey:  os.Getenv("YAC_API_KEY"),
		BaseURL: os.Getenv("YAC_BASE_URL"),
		Model:   os.Getenv("YAC_MODEL"),
	}

	// Build tools: all standard tools + delegate.
	// FilterTools will exclude SearXNG if SEARXNG_URL isn't set.
	// Memory tools are added per-chat in getOrCreate.
	allTools := tools.AllWithDelegate(adapter, 2)
	agentTools := yac.FilterTools(allTools)

	// Memory directory: use YAC_MEMORY_DIR or default to ~/.yac/memories
	memoryDir := os.Getenv("YAC_MEMORY_DIR")
	if memoryDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get home dir: %v", err)
		}
		memoryDir = filepath.Join(home, ".yac", "memories")
	}

	chats := newChatAgents(agentConfig{
		adapter:   adapter,
		tools:     agentTools,
		memoryDir: memoryDir,
	})

	// Graceful shutdown on Ctrl+C.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	log.Printf("Bot started. Model: %s @ %s", adapter.Model, adapter.BaseURL)
	log.Println("Send a message to your bot on Telegram. Press Ctrl+C to stop.")

	offset := 0
	for {
		// Check for shutdown.
		select {
		case <-ctx.Done():
			log.Println("Shutting down.")
			return
		default:
		}

		updates, err := getUpdates(ctx, token, offset)
		if err != nil {
			log.Printf("Error fetching updates: %v", err)
			continue
		}

		for _, update := range updates {
			offset = update.UpdateID + 1

			if update.Message == nil || update.Message.Text == "" {
				continue
			}

			chatID := update.Message.Chat.ID
			text := update.Message.Text
			log.Printf("[chat %d] User: %s", chatID, text)

			// Handle /start and /reset commands.
			if text == "/start" {
				_ = sendMessage(token, chatID, "Hello! I'm a yac-powered assistant. Ask me anything, or try some math!")
				continue
			}
			if text == "/reset" {
				chats.mu.Lock()
				delete(chats.agents, chatID)
				chats.mu.Unlock()
				_ = sendMessage(token, chatID, "Conversation reset. Fresh start!")
				continue
			}

			agent := chats.getOrCreate(chatID)

			// Debug: log conversation state before sending
			log.Printf("[chat %d] Agent has %d messages in history before Send", chatID, len(agent.Messages))
			for i, msg := range agent.Messages {
				preview := msg.Content
				if len(preview) > 50 {
					preview = preview[:50] + "..."
				}
				log.Printf("[chat %d]   Msg %d: [%s] %s", chatID, i, msg.Role, preview)
			}

			reply, err := agent.Send(ctx, text)
			if err != nil {
				log.Printf("[chat %d] Error: %v", chatID, err)
				_ = sendMessage(token, chatID, "Sorry, something went wrong. Try again.")
				continue
			}

			// Debug: log conversation state after sending
			log.Printf("[chat %d] Agent has %d messages in history after Send", chatID, len(agent.Messages))
			log.Printf("[chat %d] Bot: %s", chatID, reply.Content)
			if err := sendMessage(token, chatID, reply.Content); err != nil {
				log.Printf("[chat %d] Failed to send reply: %v", chatID, err)
			}
		}
	}
}
