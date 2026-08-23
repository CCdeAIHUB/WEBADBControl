package runtime

import (
	"context"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/automation"
)

func TestDelayRejectsNonNumericDuration(t *testing.T) {
	// 场景：非法 delay 参数不能被静默转换为 0 毫秒并伪装执行成功。
	executor := NewExecutor(nil)
	err := executor.ExecuteAutomationAction(context.Background(), automation.Task{}, automation.Action{
		Type: "delay", Parameters: map[string]any{"milliseconds": "invalid"},
	})
	if codeOfRuntimeError(err) != "AUTOMATION_DELAY_INVALID" {
		t.Fatalf("error = %#v, want AUTOMATION_DELAY_INVALID", err)
	}
}

func TestUnsupportedActionReturnsStructuredError(t *testing.T) {
	// 场景：尚未配置执行器的动作必须返回统一错误结构，不能泄露临时 fmt 错误。
	err := NewExecutor(nil).ExecuteAutomationAction(context.Background(), automation.Task{}, automation.Action{Type: "ai.prompt"})
	if codeOfRuntimeError(err) != "AUTOMATION_ACTION_UNSUPPORTED" {
		t.Fatalf("error = %#v, want AUTOMATION_ACTION_UNSUPPORTED", err)
	}
}

func codeOfRuntimeError(err error) string {
	if typed, ok := err.(*apperror.Error); ok {
		return typed.ErrorCode
	}
	return ""
}
