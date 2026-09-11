package device

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

type actionFallbackCaller struct {
	calls                [][]string
	accessibilityEnabled bool
}

func (c *actionFallbackCaller) Call(_ context.Context, method string, params any, target any) error {
	if method != "adb.exec" {
		return nil
	}
	args := append([]string(nil), params.(map[string]any)["args"].([]string)...)
	c.calls = append(c.calls, args)
	joined := strings.Join(args, " ")
	output := CommandOutput{}
	switch {
	case strings.Contains(joined, "shell input keyevent"):
		output.ExitCode = 255
		output.Stderr = "java.lang.SecurityException: Injecting input events requires the caller to have the INJECT_EVENTS permission."
	case strings.Contains(joined, "accessibility.status"):
		// The broadcast only schedules the command; its result is read below.
	case strings.Contains(joined, "command-results/"):
		if c.accessibilityEnabled {
			output.Stdout = `{"requestId":"test","ok":true,"result":{"enabled":true}}`
		} else {
			output.Stdout = `{"requestId":"test","ok":true,"result":{"enabled":false}}`
		}
	}
	*(target.(*CommandOutput)) = output
	return nil
}

func TestActionFallsBackToHomeIntentWhenInputInjectionIsDenied(t *testing.T) {
	// 场景：OEM 禁止 shell 注入按键时，主页键仍应通过系统 HOME Intent 工作。
	caller := &actionFallbackCaller{}
	if err := NewService(caller).Action(context.Background(), "serial-1", ActionRequest{Type: "key", Key: "HOME"}); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %#v, want input attempt and HOME intent fallback", caller.calls)
	}
	got := strings.Join(caller.calls[1], " ")
	if !strings.Contains(got, "am start -a android.intent.action.MAIN -c android.intent.category.HOME") {
		t.Fatalf("fallback call = %q", got)
	}
}

func TestActionReportsDisabledAccessibilityWithoutTriggeringCompanionAction(t *testing.T) {
	// 场景：返回键注入被拒且伴侣无障碍未启用时，只做状态探测，不得调用动作而反复拉起设置页。
	caller := &actionFallbackCaller{accessibilityEnabled: false}
	err := NewService(caller).Action(context.Background(), "serial-1", ActionRequest{Type: "key", Key: "BACK"})
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.ErrorCode != "COMPANION_ACCESSIBILITY_NOT_ENABLED" {
		t.Fatalf("err = %#v, want COMPANION_ACCESSIBILITY_NOT_ENABLED", err)
	}
	for _, call := range caller.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "accessibility.global.back") || strings.Contains(joined, "android.settings.ACCESSIBILITY_SETTINGS") {
			t.Fatalf("disabled accessibility must not trigger side effects: %#v", caller.calls)
		}
	}
}

func TestActionUsesCompanionBackWhenAccessibilityIsEnabled(t *testing.T) {
	// 场景：ADB 注入被 OEM 拒绝但伴侣无障碍可用时，返回键自动改走伴侣通道。
	caller := &actionFallbackCaller{accessibilityEnabled: true}
	if err := NewService(caller).Action(context.Background(), "serial-1", ActionRequest{Type: "key", Key: "BACK"}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, call := range caller.calls {
		if strings.Contains(strings.Join(call, " "), "accessibility.global.back") {
			found = true
		}
	}
	if !found {
		t.Fatalf("calls = %#v, want companion back operation", caller.calls)
	}
}

func TestActionDoesNotFallbackForUnrelatedADBFailure(t *testing.T) {
	// 场景：只有明确的 INJECT_EVENTS 权限异常可进入降级路径，其他错误必须原样报告。
	caller := &actionFallbackCaller{}
	caller.accessibilityEnabled = true
	// Text uses a different command that this fake leaves successful; directly
	// assert the predicate so future broad matching cannot hide transport errors.
	if isInputInjectionDenied(apperror.New("ADB_COMMAND_FAILED", "设备命令执行失败", "device.service", true).WithSuggestion("device offline")) {
		t.Fatal("unrelated ADB failure must not be treated as input permission denial")
	}
}
