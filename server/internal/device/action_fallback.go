package device

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

type companionAccessibilityStatus struct {
	OK     bool `json:"ok"`
	Result struct {
		Enabled bool `json:"enabled"`
	} `json:"result"`
}

const (
	ScreenControlScrcpy    = "scrcpy-control"
	ScreenControlCompanion = "companion-accessibility"
)

// ScreenControlTransport probes Android's shell input permission with
// KEYCODE_UNKNOWN, which Android applications ignore. This keeps scrcpy's
// low-latency pointer stream where it is supported and selects the Companion
// gesture path on OEM builds that deny INJECT_EVENTS to the shell user.
func (s *Service) ScreenControlTransport(ctx context.Context, deviceID string) (string, error) {
	_, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "input", "keyevent", "KEYCODE_UNKNOWN"))
	if err == nil {
		s.inputTransports.Store(deviceID, ScreenControlScrcpy)
		return ScreenControlScrcpy, nil
	}
	if !isInputInjectionDenied(err) {
		return ScreenControlScrcpy, err
	}
	s.inputTransports.Store(deviceID, ScreenControlCompanion)
	return ScreenControlCompanion, nil
}

func isInputInjectionDenied(err error) bool {
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.ErrorCode != "ADB_COMMAND_FAILED" {
		return false
	}
	detail := strings.ToLower(appErr.Suggestion + "\n" + appErr.Cause)
	return strings.Contains(detail, "injecting input events") && strings.Contains(detail, "inject_events")
}

func (s *Service) fallbackDeniedInput(ctx context.Context, deviceID string, request ActionRequest) (string, error) {
	if request.Type == "key" && request.Key == "HOME" {
		_, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "am", "start", "-a", "android.intent.action.MAIN", "-c", "android.intent.category.HOME"))
		return "android-home-intent", err
	}

	operation, args, ok := accessibilityFallback(request)
	if !ok {
		return "unsupported", inputPermissionError()
	}
	statusPayload, err := s.ExecuteCompanionCommand(ctx, deviceID, "android.accessibility.control", "accessibility.status", map[string]any{})
	if err != nil {
		return "companion-accessibility", err
	}
	var status companionAccessibilityStatus
	if err := json.Unmarshal([]byte(statusPayload), &status); err != nil {
		return "companion-accessibility", apperror.Wrap("COMPANION_RESULT_INVALID", "伴侣无障碍状态无法解析", "device.control", true, err)
	}
	if !status.OK || !status.Result.Enabled {
		return "companion-accessibility", apperror.New("COMPANION_ACCESSIBILITY_NOT_ENABLED", "设备禁止 ADB 控制，且伴侣无障碍尚未启用", "device.control", true).
			WithSuggestion("请在设备的无障碍设置中启用 ADBControl 伴侣；部分三星/小米设备也可启用“USB 调试（安全设置）”")
	}
	_, err = s.ExecuteCompanionCommand(ctx, deviceID, "android.accessibility.control", operation, args)
	return "companion-accessibility", err
}

func accessibilityFallback(request ActionRequest) (string, map[string]any, bool) {
	switch request.Type {
	case "key":
		operations := map[string]string{
			"BACK":       "accessibility.global.back",
			"APP_SWITCH": "accessibility.global.recents",
		}
		operation, ok := operations[request.Key]
		return operation, map[string]any{}, ok
	case "tap":
		return "accessibility.touch.tap", map[string]any{
			"x": request.X, "y": request.Y,
			"coordinateWidth": request.CoordinateWidth, "coordinateHeight": request.CoordinateHeight,
		}, true
	case "swipe":
		return "accessibility.touch.swipe", map[string]any{
			"startX": request.X, "startY": request.Y, "endX": request.EndX, "endY": request.EndY,
			"durationMs":      request.DurationMS,
			"coordinateWidth": request.CoordinateWidth, "coordinateHeight": request.CoordinateHeight,
		}, true
	default:
		return "", nil, false
	}
}

func inputPermissionError() error {
	return apperror.New("ADB_INPUT_PERMISSION_DENIED", "设备系统禁止 ADB 注入此控制操作", "device.control", true).
		WithSuggestion("请启用设备的“USB 调试（安全设置）”，或安装并启用 ADBControl 伴侣无障碍服务")
}
