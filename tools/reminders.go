package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/maccam912/yac"
)

// ReminderTask represents a Vikunja task that has fired as a reminder.
type ReminderTask struct {
	ID          int
	Title       string
	Description string
	DueDate     time.Time
}

// ReminderConfig configures the reminder poller.
type ReminderConfig struct {
	ProjectID  int
	OnReminder func(ctx context.Context, task ReminderTask)
}

// SetReminder returns a tool that creates a reminder (Vikunja task with a due date)
// in the given project.
func SetReminder(projectID int) *yac.Tool {
	return &yac.Tool{
		Name:        "set_reminder",
		Description: "Set a reminder by creating a task with a due date. When the due date passes, the reminder will fire automatically.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "Short title for the reminder.",
				},
				"due_date": map[string]any{
					"type":        "string",
					"description": "When the reminder should fire, in ISO 8601 format (e.g. '2026-03-15T14:00:00Z').",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Optional longer description or context for the reminder.",
				},
			},
			"required": []string{"title", "due_date"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Title       string `json:"title"`
				DueDate     string `json:"due_date"`
				Description string `json:"description"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if params.Title == "" {
				return "", fmt.Errorf("title is required")
			}
			if params.DueDate == "" {
				return "", fmt.Errorf("due_date is required")
			}

			// Validate the date parses
			if _, err := time.Parse(time.RFC3339, params.DueDate); err != nil {
				return "", fmt.Errorf("due_date must be valid ISO 8601 (e.g. '2026-03-15T14:00:00Z'): %w", err)
			}

			body := map[string]any{
				"title":    params.Title,
				"due_date": params.DueDate,
			}
			if params.Description != "" {
				body["description"] = params.Description
			}

			data, err := vikunjaRequest(ctx, "PUT", fmt.Sprintf("/projects/%d/tasks", projectID), body)
			if err != nil {
				return "", err
			}

			var created struct {
				ID    int    `json:"id"`
				Title string `json:"title"`
			}
			if err := json.Unmarshal(data, &created); err != nil {
				return "", fmt.Errorf("failed to parse response: %w", err)
			}

			return fmt.Sprintf("Reminder set: #%d '%s' (due %s)", created.ID, created.Title, params.DueDate), nil
		},
		ShouldInclude: vikunjaShouldInclude,
	}
}

// shouldFire returns true if a task should trigger a reminder callback.
func shouldFire(done bool, dueDate string, now time.Time) bool {
	if done {
		return false
	}
	if dueDate == "" || strings.HasPrefix(dueDate, "0001") {
		return false
	}
	t, err := time.Parse(time.RFC3339, dueDate)
	if err != nil {
		return false
	}
	return !t.After(now)
}

// StartReminderPoller launches a goroutine that polls a Vikunja project for
// overdue tasks every 5 minutes. When a task's due date is in the past, it
// calls cfg.OnReminder and then marks the task done via the API.
// The goroutine stops when ctx is cancelled.
func StartReminderPoller(ctx context.Context, cfg ReminderConfig) {
	go func() {
		pollReminders(ctx, cfg)

		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pollReminders(ctx, cfg)
			}
		}
	}()
}

func pollReminders(ctx context.Context, cfg ReminderConfig) {
	data, err := vikunjaRequest(ctx, "GET",
		fmt.Sprintf("/projects/%d/tasks?per_page=50&filter=%s",
			cfg.ProjectID, "done+%3D+false"), nil)
	if err != nil {
		log.Printf("reminder poller: failed to fetch tasks: %v", err)
		return
	}

	var tasks []struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Done        bool   `json:"done"`
		DueDate     string `json:"due_date"`
	}
	if err := json.Unmarshal(data, &tasks); err != nil {
		log.Printf("reminder poller: failed to parse tasks: %v", err)
		return
	}

	now := time.Now()
	for _, t := range tasks {
		if !shouldFire(t.Done, t.DueDate, now) {
			continue
		}

		due, _ := time.Parse(time.RFC3339, t.DueDate)
		if cfg.OnReminder != nil {
			cfg.OnReminder(ctx, ReminderTask{
				ID:          t.ID,
				Title:       t.Title,
				Description: t.Description,
				DueDate:     due,
			})
		}

		// Mark the task done
		_, err := vikunjaRequest(ctx, "POST", fmt.Sprintf("/tasks/%d", t.ID), map[string]any{"done": true})
		if err != nil {
			log.Printf("reminder poller: failed to mark task #%d done: %v", t.ID, err)
		}
	}
}
