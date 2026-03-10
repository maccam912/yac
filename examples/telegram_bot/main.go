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

type telegramSender struct {
	mu            sync.Mutex
	token         string
	defaultChatID int64
}

func newTelegramSender(token string) *telegramSender {
	return &telegramSender{token: token}
}

func (ts *telegramSender) rememberChat(chatID int64) {
	if chatID == 0 {
		return
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.defaultChatID == 0 {
		ts.defaultChatID = chatID
		log.Printf("Default Telegram chat set to %d", chatID)
	}
}

func (ts *telegramSender) defaultChat() int64 {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.defaultChatID
}

func (ts *telegramSender) send(ctx context.Context, chatID int64, text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("telegram message text is required")
	}
	if chatID == 0 {
		chatID = ts.defaultChat()
	}
	if chatID == 0 {
		return fmt.Errorf("chat_id is required until the bot has heard from its first chat")
	}

	for _, chunk := range splitTelegramMessage(text, 4096) {
		form := url.Values{
			"chat_id": {strconv.FormatInt(chatID, 10)},
			"text":    {chunk},
		}
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", ts.token),
			strings.NewReader(form.Encode()),
		)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		var apiResp struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&apiResp)
		resp.Body.Close()
		if decodeErr != nil {
			return decodeErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			if apiResp.Description != "" {
				return fmt.Errorf("telegram API returned %s: %s", resp.Status, apiResp.Description)
			}
			return fmt.Errorf("telegram API returned %s", resp.Status)
		}
		if !apiResp.OK {
			if apiResp.Description != "" {
				return fmt.Errorf("telegram API error: %s", apiResp.Description)
			}
			return fmt.Errorf("telegram API error")
		}
	}

	return nil
}

func splitTelegramMessage(text string, maxRunes int) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}

	var chunks []string
	for len(runes) > 0 {
		if len(runes) <= maxRunes {
			chunks = append(chunks, string(runes))
			break
		}

		cut := maxRunes
		for i := maxRunes - 1; i > 0; i-- {
			if runes[i] == '\n' {
				cut = i
				break
			}
		}

		chunks = append(chunks, string(runes[:cut]))
		if cut < len(runes) && runes[cut] == '\n' {
			runes = runes[cut+1:]
		} else {
			runes = runes[cut:]
		}
	}

	return chunks
}

func telegramSendTool(sender *telegramSender) *yac.Tool {
	return &yac.Tool{
		Name:        "send_telegram_message",
		Description: "Send a message to a Telegram chat using this bot. Use this for proactive alerts, reminders, and notifications. If chat_id is omitted, the first chat the bot ever heard from is used as the default destination.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"chat_id": map[string]any{
					"type":        "integer",
					"description": "Telegram chat ID to send the message to. Optional if the default chat is acceptable.",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Message text to send.",
				},
			},
			"required": []string{"text"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				ChatID int64  `json:"chat_id"`
				Text   string `json:"text"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if err := sender.send(ctx, params.ChatID, params.Text); err != nil {
				return "", err
			}
			usedChatID := params.ChatID
			if usedChatID == 0 {
				usedChatID = sender.defaultChat()
			}
			return fmt.Sprintf("Sent Telegram message to chat %d.", usedChatID), nil
		},
	}
}

func formatToolList(tools []*yac.Tool) string {
	var b strings.Builder
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		desc := strings.TrimSpace(tool.GetDescription())
		if desc == "" {
			desc = "No description provided."
		}
		b.WriteString("- ")
		b.WriteString(tool.Name)
		b.WriteString(": ")
		b.WriteString(desc)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func sendMessage(ctx context.Context, sender *telegramSender, chatID int64, text string) error {
	return sender.send(ctx, chatID, text)
}

// --- Per-chat agent management ---

type chatSession struct {
	mu    sync.Mutex
	agent *yac.Agent
}

func (cs *chatSession) send(ctx context.Context, content string, opts ...yac.SendOption) (yac.Message, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.agent.Send(ctx, content, opts...)
}

type chatAgents struct {
	mu     sync.Mutex
	agents map[int64]*chatSession
	cfg    agentConfig
}

type agentConfig struct {
	adapter           *yac.OpenAIAdapter
	baseTools         []*yac.Tool
	memoryDir         string
	telegram          *telegramSender
	reminderProjectID int
}

func newChatAgents(cfg agentConfig) *chatAgents {
	return &chatAgents{
		agents: make(map[int64]*chatSession),
		cfg:    cfg,
	}
}

func (ca *chatAgents) getOrCreate(chatID int64) *chatSession {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	if session, ok := ca.agents[chatID]; ok {
		return session
	}

	// Each chat gets its own memory directory for persistent storage.
	chatMemDir := filepath.Join(ca.cfg.memoryDir, fmt.Sprintf("chat_%d", chatID))
	memoryCfg := tools.MemoryConfig{Dir: chatMemDir}

	// 1. Give subagents access to base tools + search_memories
	subTools := make([]*yac.Tool, len(ca.cfg.baseTools))
	copy(subTools, ca.cfg.baseTools)
	subTools = append(subTools, tools.SearchMemories(memoryCfg))

	// 2. The delegate tool itself
	delegateTool := tools.Delegate(tools.DelegateConfig{
		Adapter:  ca.cfg.adapter,
		Tools:    subTools,
		MaxDepth: 1, // Only need 1 level for this pattern
	})

	// 3. Assemble top-level agent tools
	chatTools := make([]*yac.Tool, 0, 5)
	chatTools = append(chatTools, delegateTool)
	chatTools = append(chatTools, tools.MemoryTools(memoryCfg)...)
	chatTools = append(chatTools, telegramSendTool(ca.cfg.telegram))
	if ca.cfg.reminderProjectID > 0 {
		chatTools = append(chatTools, tools.SetReminder(ca.cfg.reminderProjectID))
		chatTools = append(chatTools, tools.ReminderPollerStatus())
	}

	systemTemplate := template.Must(template.New("system").Parse(`Emulation profile:
- Type: Fair Witness Bot
- Framework: Function-Epistemic Hybrid Framework
- Epistemic functions: observer, evaluator, analyst, synthesist, communicator
- Natural-language constraint: use E-Prime
- Output type: natural language
- Detail level: moderate to high
- Length: moderate to high
- Complexity: low to high, as needed
- Style: dry

Operating guidance:
- State observations, reasoning, uncertainty, and conclusions in plain language without theatrical tone.
- Prefer precise claims over persuasive framing.
- Keep responses suitable for a Telegram chat interface.
- When handling a system event or other background wake-up (e.g. a fired reminder), your plain assistant reply remains internal only.
- If the user should actually receive a Telegram notification, call send_telegram_message.
- Users can ask you to set reminders using the set_reminder tool. When a reminder fires, you will receive a system message — use send_telegram_message to notify the user.
- When the user asks to reset, start over, or clear the conversation while preserving important context, use reset_conversation.
{{if .ReminderPollerActive}}
Reminder poller:
- A background poller is actively monitoring Vikunja project {{.ReminderProjectID}} for overdue tasks every 5 minutes.
- When a reminder fires, the user is notified automatically via Telegram and the task is marked done.
- You can set new reminders with the set_reminder tool.
{{end}}
Task orchestration:
- You are an orchestrator. You have NO tools for factual lookup, math, or web search.
- You DO have access to memory tools, reminder tools, and a telegram message sender.
- Any time the user asks for information, calculation, or external search, you MUST use the 'delegate' tool to spawn subagents.
- Subagents run concurrently and have access to all standard tools PLUS the search_memories tool, but ZERO CONTEXT of this conversation.
- When delegating, you MUST provide every detail, piece of context, and instruction the subagent needs in its task description.
- Synthesize the subagents' final reports into a coherent response for the user.
Current date and time:
- Day of week: {{.DayOfWeek}}
- Date/time (UTC): {{.DateTime}}

Available tools:
{{.ToolList}}
{{if .EssentialMemories}}

Essential memories:
{{.EssentialMemories}}{{end}}`))

	agent := &yac.Agent{
		Adapter: ca.cfg.adapter,
		SystemPrompt: yac.TemplatePrompt(systemTemplate, func() any {
			now := time.Now().UTC()
			essentials := tools.EssentialMemories(chatMemDir)
			var essentialStr string
			for _, title := range essentials {
				essentialStr += "- " + title + "\n"
			}
			data := map[string]any{
				"DateTime":             now.Format("2006-01-02 15:04:05 UTC"),
				"DayOfWeek":            now.Weekday().String(),
				"EssentialMemories":    essentialStr,
				"ToolList":             formatToolList(chatTools),
				"ChatID":               strconv.FormatInt(chatID, 10),
				"ReminderPollerActive": ca.cfg.reminderProjectID > 0,
				"ReminderProjectID":    strconv.Itoa(ca.cfg.reminderProjectID),
			}
			return data
		}),
		Tools:          chatTools,
		ContextLength:  8192,
		AggressiveTrim: true,
		MaxToolRounds:  35,
		PostChatAction: yac.StaticPrompt(
			"[SYSTEM] Review the conversation above. If the user shared any new facts, preferences, " +
				"or information worth remembering, save or update memories now using your memory tools. " +
				"If nothing noteworthy was said, do nothing. Do not respond to the user — this is an " +
				"internal housekeeping step.",
		),
	}
	agent.Tools = append(chatTools, tools.AgentTools(agent, chatMemDir)...)
	session := &chatSession{agent: agent}
	ca.agents[chatID] = session
	return session
}

func main() {
	_ = godotenv.Load()

	// Route standard log output through yac's log buffer so the agent
	// can introspect application logs (reminders, errors, etc.).
	log.SetOutput(yac.LogWriter())

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required. Copy .env.example to .env and fill it in.")
	}

	adapter := &yac.OpenAIAdapter{
		APIKey:  os.Getenv("YAC_API_KEY"),
		BaseURL: os.Getenv("YAC_BASE_URL"),
		Model:   os.Getenv("YAC_MODEL"),
	}
	telegram := newTelegramSender(token)

	var reminderProjectID int
	if s := os.Getenv("VIKUNJA_REMINDER_PROJECT_ID"); s != "" {
		if id, err := strconv.Atoi(s); err == nil {
			reminderProjectID = id
		}
	}

	// Build base tools for subagents.
	// FilterTools will exclude SearXNG/Vikunja if env vars aren't set.
	baseTools := yac.FilterTools(tools.All())

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
		adapter:           adapter,
		baseTools:         baseTools,
		memoryDir:         memoryDir,
		telegram:          telegram,
		reminderProjectID: reminderProjectID,
	})

	// Graceful shutdown on Ctrl+C.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Start reminder poller if configured.
	if reminderProjectID > 0 {
		tools.StartReminderPoller(ctx, tools.ReminderConfig{
			ProjectID: reminderProjectID,
			OnReminder: func(ctx context.Context, task tools.ReminderTask) {
				chatID := telegram.defaultChat()
				if chatID == 0 {
					log.Printf("[reminder] Task #%d '%s' fired but no chat available yet", task.ID, task.Title)
					return
				}
				msg := fmt.Sprintf("[REMINDER] %s (was due %s)",
					task.Title, task.DueDate.Format("2006-01-02 15:04"))
				if task.Description != "" {
					msg += "\n" + task.Description
				}
				if err := sendMessage(ctx, telegram, chatID, msg); err != nil {
					log.Printf("[reminder] Failed to send reminder for task #%d: %v", task.ID, err)
				}
			},
		})
		log.Printf("Reminder poller started for Vikunja project %d", reminderProjectID)
	}

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
			telegram.rememberChat(chatID)
			log.Printf("[chat %d] User: %s", chatID, text)

			// Handle /start and /reset commands.
			if text == "/start" {
				_ = sendMessage(ctx, telegram, chatID, "Hello! I'm a yac-powered assistant. Ask me anything or use /reset for a fresh start.")
				continue
			}
			if text == "/reset" {
				chats.mu.Lock()
				delete(chats.agents, chatID)
				chats.mu.Unlock()
				_ = sendMessage(ctx, telegram, chatID, "Conversation reset. Fresh start!")
				continue
			}

			session := chats.getOrCreate(chatID)
			reply, err := session.send(ctx, text)
			if err != nil {
				log.Printf("[chat %d] Error: %v", chatID, err)
				_ = sendMessage(ctx, telegram, chatID, "Sorry, something went wrong. Try again.")
				continue
			}

			log.Printf("[chat %d] Bot: %s", chatID, reply.Content)
			if err := sendMessage(ctx, telegram, chatID, reply.Content); err != nil {
				log.Printf("[chat %d] Failed to send reply: %v", chatID, err)
			}
		}
	}
}
