package automation

import (
	"fmt"
	"strings"
	"time"
)

type Permissions struct {
	AllowADB       bool `json:"allowAdb"`
	AllowShell     bool `json:"allowShell"`
	AllowCompanion bool `json:"allowCompanion"`
	AllowAI        bool `json:"allowAi"`
}

type Trigger struct {
	ID              string         `json:"id,omitempty"`
	Type            string         `json:"type"`
	At              string         `json:"at,omitempty"`
	Cron            string         `json:"cron,omitempty"`
	IntervalSeconds int            `json:"intervalSeconds,omitempty"`
	Parameters      map[string]any `json:"parameters,omitempty"`
}

type Action struct {
	Type       string         `json:"type"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type Task struct {
	SchemaVersion     int         `json:"schemaVersion"`
	ID                string      `json:"id"`
	Name              string      `json:"name"`
	Description       string      `json:"description,omitempty"`
	DeviceID          string      `json:"deviceId,omitempty"`
	Enabled           bool        `json:"enabled"`
	ConcurrencyPolicy string      `json:"concurrencyPolicy"`
	Permissions       Permissions `json:"permissions"`
	Triggers          []Trigger   `json:"triggers"`
	Actions           []Action    `json:"actions"`
	CreatedAt         time.Time   `json:"createdAt"`
	UpdatedAt         time.Time   `json:"updatedAt"`
}

func Validate(task Task) error {
	if strings.TrimSpace(task.Name) == "" {
		return fmt.Errorf("task name is required")
	}
	if len(task.Triggers) == 0 || len(task.Actions) == 0 {
		return fmt.Errorf("at least one trigger and action are required")
	}
	for _, trigger := range task.Triggers {
		switch trigger.Type {
		case "manual", "daily", "weekly", "interval", "cron", "condition":
		default:
			return fmt.Errorf("unsupported trigger %q", trigger.Type)
		}
	}
	for _, action := range task.Actions {
		switch action.Type {
		case "log", "delay", "device.action", "adb.command", "adb.shell", "companion.call", "ai.prompt", "condition", "parallel", "loop":
		default:
			return fmt.Errorf("unsupported action %q", action.Type)
		}
		if strings.HasPrefix(action.Type, "adb.") && !task.Permissions.AllowADB {
			return fmt.Errorf("action %s requires allowAdb", action.Type)
		}
		if action.Type == "adb.shell" && !task.Permissions.AllowShell {
			return fmt.Errorf("adb.shell requires allowShell")
		}
		if action.Type == "companion.call" && !task.Permissions.AllowCompanion {
			return fmt.Errorf("companion.call requires allowCompanion")
		}
		if action.Type == "ai.prompt" && !task.Permissions.AllowAI {
			return fmt.Errorf("ai.prompt requires allowAi")
		}
	}
	return nil
}

type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunPaused    RunStatus = "paused"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunStopped   RunStatus = "stopped"
	RunSkipped   RunStatus = "skipped"
)

type Run struct {
	ID             string    `json:"id"`
	TaskID         string    `json:"taskId"`
	Status         RunStatus `json:"status"`
	Progress       float64   `json:"progress"`
	CompletedSteps int       `json:"completedSteps"`
	TotalSteps     int       `json:"totalSteps"`
	ErrorCode      string    `json:"errorCode,omitempty"`
	StartedAt      time.Time `json:"startedAt,omitempty"`
	FinishedAt     time.Time `json:"finishedAt,omitempty"`
}

var transitions = map[RunStatus]map[RunStatus]bool{
	RunQueued:  {RunRunning: true, RunStopped: true, RunSkipped: true},
	RunRunning: {RunPaused: true, RunSucceeded: true, RunFailed: true, RunStopped: true},
	RunPaused:  {RunRunning: true, RunFailed: true, RunStopped: true},
}

func (run *Run) Transition(next RunStatus) error {
	if !transitions[run.Status][next] {
		return fmt.Errorf("illegal run transition %s -> %s", run.Status, next)
	}
	run.Status = next
	if next == RunRunning && run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	if next == RunSucceeded || next == RunFailed || next == RunStopped || next == RunSkipped {
		run.FinishedAt = time.Now().UTC()
	}
	return nil
}
