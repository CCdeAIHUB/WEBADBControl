package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/automation"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
)

type Executor struct{ devices *device.Service }

func NewExecutor(devices *device.Service) *Executor { return &Executor{devices: devices} }

func (e *Executor) ExecuteAutomationAction(ctx context.Context, task automation.Task, action automation.Action) error {
	switch action.Type {
	case "log":
		return nil
	case "delay":
		milliseconds, _ := number(action.Parameters["milliseconds"])
		if milliseconds < 0 || milliseconds > 86_400_000 {
			return fmt.Errorf("delay is outside allowed range")
		}
		timer := time.NewTimer(time.Duration(milliseconds) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	case "device.action":
		encoded, _ := json.Marshal(action.Parameters)
		var request device.ActionRequest
		if err := json.Unmarshal(encoded, &request); err != nil {
			return err
		}
		return e.devices.Action(ctx, task.DeviceID, request)
	case "adb.command":
		args, err := stringSlice(action.Parameters["args"])
		if err != nil {
			return err
		}
		_, err = e.devices.Exec(ctx, device.DeviceArgs(task.DeviceID, args...))
		return err
	case "adb.shell":
		command, ok := action.Parameters["command"].(string)
		if !ok || command == "" {
			return fmt.Errorf("adb.shell command is required")
		}
		_, err := e.devices.Exec(ctx, device.DeviceArgs(task.DeviceID, "shell", command))
		return err
	case "companion.call":
		return e.devices.Core().Call(ctx, "device.invoke", map[string]any{
			"deviceId":     task.DeviceID,
			"capabilityId": action.Parameters["capabilityId"],
			"operation":    action.Parameters["operation"],
			"args":         action.Parameters["args"],
		}, nil)
	default:
		return fmt.Errorf("action %s requires a configured extension executor", action.Type)
	}
}

func number(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case string:
		parsed, err := strconv.Atoi(typed)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func stringSlice(value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		if direct, ok := value.([]string); ok {
			return direct, nil
		}
		return nil, fmt.Errorf("args must be a string array")
	}
	result := make([]string, len(items))
	for index, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("args[%d] must be a string", index)
		}
		result[index] = text
	}
	return result, nil
}
