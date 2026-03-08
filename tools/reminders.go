package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ReminderTask represents a Vikunja task that has been identified as a due reminder.
type ReminderTask struct {
	ID          int
	Title       string
	Description string
	DueDate     string
	RepeatAfter int // seconds, 0 = one-off
	ChatID      int64
}

// ReminderConfig configures the reminder poller.
type ReminderConfig struct {
	ProjectID    int                                          // Vikunja project to poll for reminders
	FiredFile    string                                       // Path to fired.json
	PollInterval time.Duration                                // How often to check (default 60s)
	OnReminder   func(ctx context.Context, task ReminderTask) // Callback when a reminder fires
}

// ReminderPoller checks a Vikunja project for overdue tasks and fires reminders.
type ReminderPoller struct {
	cfg ReminderConfig
}

// NewReminderPoller creates a new poller with the given config.
func NewReminderPoller(cfg ReminderConfig) *ReminderPoller {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 60 * time.Second
	}
	return &ReminderPoller{cfg: cfg}
}

// Start runs the polling loop. It blocks until ctx is cancelled.
func (rp *ReminderPoller) Start(ctx context.Context) {
	ticker := time.NewTicker(rp.cfg.PollInterval)
	defer ticker.Stop()

	// Poll immediately on start.
	rp.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rp.poll(ctx)
		}
	}
}

type vikunjaTaskRaw struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	DueDate     string `json:"due_date"`
	Done        bool   `json:"done"`
	RepeatAfter int    `json:"repeat_after"`
}

func (rp *ReminderPoller) poll(ctx context.Context) {
	path := fmt.Sprintf("/projects/%d/tasks?per_page=200", rp.cfg.ProjectID)
	data, err := vikunjaRequest(ctx, "GET", path, nil)
	if err != nil {
		log.Printf("[reminders] Failed to fetch tasks: %v", err)
		return
	}

	var tasks []vikunjaTaskRaw
	if err := json.Unmarshal(data, &tasks); err != nil {
		log.Printf("[reminders] Failed to parse tasks: %v", err)
		return
	}

	fired := rp.loadFired()
	now := time.Now()
	taskIDs := make(map[string]bool)
	changed := false

	for _, t := range tasks {
		taskIDStr := strconv.Itoa(t.ID)
		taskIDs[taskIDStr] = true

		if t.Done {
			continue
		}
		if t.DueDate == "" || strings.HasPrefix(t.DueDate, "0001") {
			continue
		}

		dueTime, err := time.Parse(time.RFC3339, t.DueDate)
		if err != nil {
			// Try alternate format without timezone.
			dueTime, err = time.Parse("2006-01-02T15:04:05", t.DueDate)
			if err != nil {
				continue
			}
		}

		if !shouldFire(now, t.DueDate, fired[taskIDStr]) {
			continue
		}

		// Extract chat_id from description.
		chatID := extractChatID(t.Description)

		reminder := ReminderTask{
			ID:          t.ID,
			Title:       t.Title,
			Description: t.Description,
			DueDate:     t.DueDate,
			RepeatAfter: t.RepeatAfter,
			ChatID:      chatID,
		}

		if rp.cfg.OnReminder != nil {
			rp.cfg.OnReminder(ctx, reminder)
		}

		// Mark task done via Vikunja API.
		_, markErr := vikunjaRequest(ctx, "POST", fmt.Sprintf("/tasks/%d", t.ID), map[string]any{"done": true})
		if markErr != nil {
			log.Printf("[reminders] Failed to mark task #%d done: %v", t.ID, markErr)
		} else {
			log.Printf("[reminders] Fired reminder for task #%d (%s), due %s", t.ID, t.Title, dueTime.Format(time.RFC3339))
		}

		fired[taskIDStr] = t.DueDate
		changed = true
	}

	// Prune entries for tasks no longer in the list.
	for id := range fired {
		if !taskIDs[id] {
			delete(fired, id)
			changed = true
		}
	}

	if changed {
		rp.saveFired(fired)
	}
}

// shouldFire determines whether a task should fire based on current time, due date, and fired state.
func shouldFire(now time.Time, dueDate string, firedDueDate string) bool {
	dueTime, err := time.Parse(time.RFC3339, dueDate)
	if err != nil {
		dueTime, err = time.Parse("2006-01-02T15:04:05", dueDate)
		if err != nil {
			return false
		}
	}

	if now.Before(dueTime) {
		return false
	}

	// Not yet fired, or fired with a different (older) due date.
	return firedDueDate != dueDate
}

var chatIDRegexp = regexp.MustCompile(`(?m)^chat_id:(\d+)\s*$`)

// extractChatID scans a task description for a line like "chat_id:12345".
func extractChatID(description string) int64 {
	matches := chatIDRegexp.FindStringSubmatch(description)
	if len(matches) < 2 {
		return 0
	}
	id, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func (rp *ReminderPoller) loadFired() map[string]string {
	return loadFiredFile(rp.cfg.FiredFile)
}

func (rp *ReminderPoller) saveFired(fired map[string]string) {
	saveFiredFile(rp.cfg.FiredFile, fired)
}

func loadFiredFile(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[string]string)
	}
	var fired map[string]string
	if err := json.Unmarshal(data, &fired); err != nil {
		return make(map[string]string)
	}
	return fired
}

func saveFiredFile(path string, fired map[string]string) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("[reminders] Failed to create directory %s: %v", dir, err)
		return
	}
	data, err := json.Marshal(fired)
	if err != nil {
		log.Printf("[reminders] Failed to marshal fired data: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("[reminders] Failed to write fired file: %v", err)
	}
}
