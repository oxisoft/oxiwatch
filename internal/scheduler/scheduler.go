package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type Task func(ctx context.Context) error

type taskType int

const (
	taskTypeDaily taskType = iota
	taskTypeMonthly
)

type Scheduler struct {
	logger *slog.Logger
	tasks  []scheduledTask
}

type scheduledTask struct {
	name     string
	task     Task
	hour     int
	minute   int
	location *time.Location
	lastRun  time.Time
	taskType taskType
}

func New(logger *slog.Logger) *Scheduler {
	return &Scheduler{
		logger: logger,
	}
}

func (s *Scheduler) AddDailyTask(name string, timeStr string, timezone string, task Task) error {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return err
	}

	hour, minute, err := parseTime(timeStr)
	if err != nil {
		return err
	}

	s.tasks = append(s.tasks, scheduledTask{
		name:     name,
		task:     task,
		hour:     hour,
		minute:   minute,
		location: loc,
		taskType: taskTypeDaily,
	})

	return nil
}

func (s *Scheduler) AddMonthlyTask(name string, timeStr string, timezone string, task Task) error {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return err
	}

	hour, minute, err := parseTime(timeStr)
	if err != nil {
		return err
	}

	s.tasks = append(s.tasks, scheduledTask{
		name:     name,
		task:     task,
		hour:     hour,
		minute:   minute,
		location: loc,
		taskType: taskTypeMonthly,
	})

	return nil
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkTasks(ctx)
		}
	}
}

func (s *Scheduler) checkTasks(ctx context.Context) {
	now := time.Now()

	for i := range s.tasks {
		task := &s.tasks[i]
		localNow := now.In(task.location)

		if localNow.Hour() == task.hour && localNow.Minute() == task.minute {
			today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, task.location)
			if task.lastRun.Before(today) {
				if task.taskType == taskTypeMonthly && !isLastDayOfMonth(localNow) {
					continue
				}

				s.logger.Info("running scheduled task", "name", task.name)
				if err := task.task(ctx); err != nil {
					s.logger.Error("scheduled task failed", "name", task.name, "error", err)
				} else {
					s.logger.Info("scheduled task completed", "name", task.name)
				}
				task.lastRun = now
			}
		}
	}
}

func isLastDayOfMonth(t time.Time) bool {
	tomorrow := t.AddDate(0, 0, 1)
	return tomorrow.Month() != t.Month()
}

func parseTime(timeStr string) (hour, minute int, err error) {
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return 0, 0, err
	}
	return t.Hour(), t.Minute(), nil
}
