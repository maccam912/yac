package tools

import (
	"testing"
	"time"
)

func TestShouldFire(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		done    bool
		dueDate string
		want    bool
	}{
		{
			name:    "past due fires",
			done:    false,
			dueDate: "2026-03-09T11:00:00Z",
			want:    true,
		},
		{
			name:    "exactly now fires",
			done:    false,
			dueDate: "2026-03-09T12:00:00Z",
			want:    true,
		},
		{
			name:    "future does not fire",
			done:    false,
			dueDate: "2026-03-09T13:00:00Z",
			want:    false,
		},
		{
			name:    "already done does not fire",
			done:    true,
			dueDate: "2026-03-09T11:00:00Z",
			want:    false,
		},
		{
			name:    "empty due date does not fire",
			done:    false,
			dueDate: "",
			want:    false,
		},
		{
			name:    "zero date does not fire",
			done:    false,
			dueDate: "0001-01-01T00:00:00Z",
			want:    false,
		},
		{
			name:    "invalid date does not fire",
			done:    false,
			dueDate: "not-a-date",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldFire(tt.done, tt.dueDate, now)
			if got != tt.want {
				t.Errorf("shouldFire(done=%v, dueDate=%q) = %v, want %v",
					tt.done, tt.dueDate, got, tt.want)
			}
		})
	}
}
