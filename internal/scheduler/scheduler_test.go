package scheduler

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNew(t *testing.T) {
	logger := testLogger()
	s := New(logger)
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.logger != logger {
		t.Error("logger not stored")
	}
	if len(s.tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(s.tasks))
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantHour   int
		wantMinute int
		wantErr    bool
	}{
		{"midnight", "00:00", 0, 0, false},
		{"morning", "09:05", 9, 5, false},
		{"noon", "12:00", 12, 0, false},
		{"afternoon", "13:45", 13, 45, false},
		{"end of day", "23:59", 23, 59, false},
		{"leading zero minute", "08:07", 8, 7, false},
		{"empty", "", 0, 0, true},
		{"hour out of range", "24:00", 0, 0, true},
		{"minute out of range", "10:60", 0, 0, true},
		{"non numeric", "ab:cd", 0, 0, true},
		{"missing colon", "1200", 0, 0, true},
		{"single field", "12", 0, 0, true},
		{"with seconds", "12:00:00", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hour, minute, err := parseTime(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got hour=%d minute=%d", tt.input, hour, minute)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if hour != tt.wantHour || minute != tt.wantMinute {
				t.Errorf("parseTime(%q) = (%d, %d), want (%d, %d)", tt.input, hour, minute, tt.wantHour, tt.wantMinute)
			}
		})
	}
}

func TestIsLastDayOfMonth(t *testing.T) {
	utc := time.UTC
	tests := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"jan 31 is last", time.Date(2024, 1, 31, 12, 0, 0, 0, utc), true},
		{"jan 30 not last", time.Date(2024, 1, 30, 12, 0, 0, 0, utc), false},
		{"feb 29 leap year last", time.Date(2024, 2, 29, 0, 0, 0, 0, utc), true},
		{"feb 28 leap year not last", time.Date(2024, 2, 28, 0, 0, 0, 0, utc), false},
		{"feb 28 non leap year last", time.Date(2023, 2, 28, 0, 0, 0, 0, utc), true},
		{"apr 30 last", time.Date(2024, 4, 30, 23, 59, 0, 0, utc), true},
		{"apr 29 not last", time.Date(2024, 4, 29, 23, 59, 0, 0, utc), false},
		{"dec 31 last of year", time.Date(2024, 12, 31, 0, 0, 0, 0, utc), true},
		{"dec 30 not last", time.Date(2024, 12, 30, 0, 0, 0, 0, utc), false},
		{"first of month", time.Date(2024, 6, 1, 0, 0, 0, 0, utc), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLastDayOfMonth(tt.t); got != tt.want {
				t.Errorf("isLastDayOfMonth(%v) = %v, want %v", tt.t, got, tt.want)
			}
		})
	}
}

func noopTask(ctx context.Context) error { return nil }

func TestAddDailyTask(t *testing.T) {
	tests := []struct {
		name     string
		timeStr  string
		timezone string
		wantErr  bool
	}{
		{"valid utc", "09:30", "UTC", false},
		{"valid named zone", "23:00", "Europe/Berlin", false},
		{"valid local", "00:00", "Local", false},
		{"invalid time", "99:99", "UTC", true},
		{"invalid timezone", "09:30", "Not/AZone", true},
		{"empty time", "", "UTC", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(testLogger())
			err := s.AddDailyTask("daily", tt.timeStr, tt.timezone, noopTask)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if len(s.tasks) != 0 {
					t.Errorf("task should not be added on error, got %d tasks", len(s.tasks))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(s.tasks) != 1 {
				t.Fatalf("expected 1 task, got %d", len(s.tasks))
			}
			got := s.tasks[0]
			if got.taskType != taskTypeDaily {
				t.Errorf("taskType = %v, want daily", got.taskType)
			}
			if got.name != "daily" {
				t.Errorf("name = %q, want daily", got.name)
			}
			if got.location == nil {
				t.Error("location is nil")
			}
		})
	}
}

func TestAddMonthlyTask(t *testing.T) {
	tests := []struct {
		name     string
		timeStr  string
		timezone string
		wantErr  bool
	}{
		{"valid", "08:00", "UTC", false},
		{"invalid time", "ab:cd", "UTC", true},
		{"invalid timezone", "08:00", "Mars/Phobos", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(testLogger())
			err := s.AddMonthlyTask("monthly", tt.timeStr, tt.timezone, noopTask)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(s.tasks) != 1 {
				t.Fatalf("expected 1 task, got %d", len(s.tasks))
			}
			if s.tasks[0].taskType != taskTypeMonthly {
				t.Errorf("taskType = %v, want monthly", s.tasks[0].taskType)
			}
		})
	}
}

func TestAddTaskStoresHourMinute(t *testing.T) {
	s := New(testLogger())
	if err := s.AddDailyTask("t", "14:25", "UTC", noopTask); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.tasks[0].hour != 14 || s.tasks[0].minute != 25 {
		t.Errorf("hour/minute = %d:%d, want 14:25", s.tasks[0].hour, s.tasks[0].minute)
	}
}

func TestMultipleTasksAppended(t *testing.T) {
	s := New(testLogger())
	if err := s.AddDailyTask("a", "01:00", "UTC", noopTask); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMonthlyTask("b", "02:00", "UTC", noopTask); err != nil {
		t.Fatal(err)
	}
	if len(s.tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(s.tasks))
	}
	if s.tasks[0].taskType != taskTypeDaily || s.tasks[1].taskType != taskTypeMonthly {
		t.Error("task order/type mismatch")
	}
}

// TestStartStopsOnContextCancel verifies Start returns promptly when its
// context is cancelled, without waiting for the 30s ticker. This does not
// exercise checkTasks (which depends on wall-clock time) but confirms the
// loop honours cancellation.
func TestStartStopsOnContextCancel(t *testing.T) {
	s := New(testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}
