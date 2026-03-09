// Example: Agent with reminder tools and a reminder poller.
//
// Demonstrates setting up a set_reminder tool (backed by Vikunja) and
// a StartReminderPoller that fires a callback when reminders come due.
// Requires VIKUNJA_URL and VIKUNJA_API_KEY environment variables.
//
// Usage:
//
//	go run ./examples/reminders/
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/maccam912/yac"
	"github.com/maccam912/yac/tools"
)

func main() {
	_ = godotenv.Load()

	projectID := 1
	if s := os.Getenv("VIKUNJA_REMINDER_PROJECT_ID"); s != "" {
		if id, err := strconv.Atoi(s); err == nil {
			projectID = id
		}
	}

	adapter := &yac.OpenAIAdapter{
		APIKey:  os.Getenv("YAC_API_KEY"),
		BaseURL: os.Getenv("YAC_BASE_URL"),
		Model:   os.Getenv("YAC_MODEL"),
	}

	reminderTool := tools.SetReminder(projectID)

	agent := yac.Agent{
		Adapter: adapter,
		SystemPrompt: yac.StaticPrompt(
			"You are a helpful assistant. You can set reminders for the user using the set_reminder tool. " +
				"When asked to set a reminder, use the tool with a title and ISO 8601 due date.",
		),
		Tools: yac.FilterTools([]*yac.Tool{
			tools.Calculator(),
			reminderTool,
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the reminder poller — when a reminder fires, just log it.
	// In a real app, you'd send a notification (e.g. Telegram message).
	tools.StartReminderPoller(ctx, tools.ReminderConfig{
		ProjectID: projectID,
		OnReminder: func(ctx context.Context, task tools.ReminderTask) {
			log.Printf("[REMINDER] Task #%d '%s' is due (was due %s)",
				task.ID, task.Title, task.DueDate.Format("2006-01-02 15:04:05"))
		},
	})

	reply, err := agent.Send(ctx, "Set a reminder titled 'Check the oven' for 5 minutes from now.")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant: %s\n", reply.Content)

	// Show conversation to see the tool call.
	fmt.Println("\n=== Conversation History ===")
	for i, msg := range agent.Messages {
		switch msg.Role {
		case "tool":
			fmt.Printf("[%d] %s (call_id=%s): %s\n", i, msg.Role, msg.ToolCallID, msg.Content)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					fmt.Printf("[%d] %s: calls %s(%s)\n", i, msg.Role, tc.Function.Name, tc.Function.Arguments)
				}
			} else {
				fmt.Printf("[%d] %s: %s\n", i, msg.Role, msg.Content)
			}
		default:
			fmt.Printf("[%d] %s: %s\n", i, msg.Role, msg.Content)
		}
	}
}
