package device

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type ActionRequest struct {
	Type       string `json:"type"`
	X          int    `json:"x,omitempty"`
	Y          int    `json:"y,omitempty"`
	EndX       int    `json:"endX,omitempty"`
	EndY       int    `json:"endY,omitempty"`
	DurationMS int    `json:"durationMs,omitempty"`
	Key        string `json:"key,omitempty"`
	Text       string `json:"text,omitempty"`
}

func ActionArguments(deviceID string, action ActionRequest) ([]string, error) {
	if strings.TrimSpace(deviceID) == "" || strings.ContainsRune(deviceID, 0) {
		return nil, fmt.Errorf("device id is invalid")
	}
	base := []string{"-s", deviceID, "shell", "input"}
	coordinate := func(value int) string { return strconv.Itoa(value) }
	switch action.Type {
	case "tap":
		if action.X < 0 || action.Y < 0 {
			return nil, fmt.Errorf("tap coordinates must be non-negative")
		}
		return append(base, "tap", coordinate(action.X), coordinate(action.Y)), nil
	case "swipe":
		if action.X < 0 || action.Y < 0 || action.EndX < 0 || action.EndY < 0 {
			return nil, fmt.Errorf("swipe coordinates must be non-negative")
		}
		duration := action.DurationMS
		if duration <= 0 {
			duration = 300
		}
		if duration > 10_000 {
			return nil, fmt.Errorf("swipe duration exceeds limit")
		}
		return append(base, "swipe", coordinate(action.X), coordinate(action.Y), coordinate(action.EndX), coordinate(action.EndY), coordinate(duration)), nil
	case "key":
		allowed := map[string]bool{"HOME": true, "BACK": true, "APP_SWITCH": true, "POWER": true, "VOLUME_UP": true, "VOLUME_DOWN": true, "ENTER": true, "DEL": true}
		if !allowed[action.Key] {
			return nil, fmt.Errorf("unsupported key %q", action.Key)
		}
		return append(base, "keyevent", action.Key), nil
	case "text":
		if !utf8.ValidString(action.Text) || strings.ContainsRune(action.Text, 0) {
			return nil, fmt.Errorf("input text is invalid")
		}
		return append(base, "text", strings.ReplaceAll(action.Text, " ", "%s")), nil
	default:
		return nil, fmt.Errorf("unsupported action %q", action.Type)
	}
}
