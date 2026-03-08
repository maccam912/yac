package tools

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractChatID(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        int64
	}{
		{
			name:        "simple",
			description: "Some reminder\nchat_id:12345",
			want:        12345,
		},
		{
			name:        "with surrounding text",
			description: "Buy groceries\nchat_id:99887766\nDon't forget milk",
			want:        99887766,
		},
		{
			name:        "at start of description",
			description: "chat_id:42\nReminder text here",
			want:        42,
		},
		{
			name:        "no chat_id",
			description: "Just a plain task description",
			want:        0,
		},
		{
			name:        "empty description",
			description: "",
			want:        0,
		},
		{
			name:        "chat_id in middle of line is not matched",
			description: "text before chat_id:123 text after",
			want:        0,
		},
		{
			name:        "only line",
			description: "chat_id:555",
			want:        555,
		},
		{
			name:        "negative chat_id (group chat)",
			description: "Reminder\nchat_id:-1001234567890\nDetails",
			want:        -1001234567890,
		},
		{
			name:        "negative chat_id only line",
			description: "chat_id:-42",
			want:        -42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractChatID(tt.description)
			if got != tt.want {
				t.Errorf("extractChatID(%q) = %d, want %d", tt.description, got, tt.want)
			}
		})
	}
}

func TestFiredFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fired.json")

	// Load from non-existent file returns empty map.
	fired := loadFiredFile(path)
	if len(fired) != 0 {
		t.Fatalf("expected empty map, got %v", fired)
	}

	// Save and reload.
	fired["123"] = "2026-03-08T15:00:00Z"
	fired["456"] = "2026-03-08T17:00:00Z"
	saveFiredFile(path, fired)

	loaded := loadFiredFile(path)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded))
	}
	if loaded["123"] != "2026-03-08T15:00:00Z" {
		t.Errorf("entry 123 = %q, want 2026-03-08T15:00:00Z", loaded["123"])
	}
	if loaded["456"] != "2026-03-08T17:00:00Z" {
		t.Errorf("entry 456 = %q, want 2026-03-08T17:00:00Z", loaded["456"])
	}

	// Prune: remove an entry and save.
	delete(loaded, "123")
	saveFiredFile(path, loaded)

	reloaded := loadFiredFile(path)
	if len(reloaded) != 1 {
		t.Fatalf("expected 1 entry after prune, got %d", len(reloaded))
	}
	if _, ok := reloaded["123"]; ok {
		t.Error("entry 123 should have been pruned")
	}
}

func TestFiredFileNestedDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "fired.json")

	fired := map[string]string{"1": "2026-01-01T00:00:00Z"}
	saveFiredFile(path, fired)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fired file not created in nested dir: %v", err)
	}

	loaded := loadFiredFile(path)
	if loaded["1"] != "2026-01-01T00:00:00Z" {
		t.Errorf("unexpected value: %q", loaded["1"])
	}
}

func TestShouldFire(t *testing.T) {
	now := time.Date(2026, 3, 8, 16, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		dueDate      string
		firedDueDate string
		want         bool
	}{
		{
			name:         "overdue and not fired",
			dueDate:      "2026-03-08T15:00:00Z",
			firedDueDate: "",
			want:         true,
		},
		{
			name:         "overdue but already fired with same due date",
			dueDate:      "2026-03-08T15:00:00Z",
			firedDueDate: "2026-03-08T15:00:00Z",
			want:         false,
		},
		{
			name:         "overdue, fired with older due date (recurring advanced)",
			dueDate:      "2026-03-08T15:00:00Z",
			firedDueDate: "2026-03-07T15:00:00Z",
			want:         true,
		},
		{
			name:         "not yet due",
			dueDate:      "2026-03-08T17:00:00Z",
			firedDueDate: "",
			want:         false,
		},
		{
			name:         "exactly now",
			dueDate:      "2026-03-08T16:00:00Z",
			firedDueDate: "",
			want:         true,
		},
		{
			name:         "invalid date",
			dueDate:      "not-a-date",
			firedDueDate: "",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldFire(now, tt.dueDate, tt.firedDueDate)
			if got != tt.want {
				t.Errorf("shouldFire(now, %q, %q) = %v, want %v", tt.dueDate, tt.firedDueDate, got, tt.want)
			}
		})
	}
}
