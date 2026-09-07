package device

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

type companionStatusCaller struct {
	calls [][]string
}

func (c *companionStatusCaller) Call(_ context.Context, method string, params any, target any) error {
	if method != "adb.exec" {
		return nil
	}
	args := params.(map[string]any)["args"].([]string)
	c.calls = append(c.calls, append([]string(nil), args...))
	output := CommandOutput{}
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "pm list packages com.adbcontrol.companion"):
		output.Stdout = "package:com.adbcontrol.companion\n"
	case strings.Contains(joined, "cat /sdcard/Android/data/com.adbcontrol.companion/files/command-results/"):
		output.Stdout = `{"requestId":"test","ok":true,"result":{"enabled":true}}`
	}
	*(target.(*CommandOutput)) = output
	return nil
}

func TestCompanionStatusUsesSideEffectFreeADBProbe(t *testing.T) {
	// 场景：坏屏/外接屏设备上，状态刷新只能探测安装与 broadcast 可达性，不能通过 monkey/am start 反复调起 App。
	caller := &companionStatusCaller{}
	status, err := NewService(caller).CompanionStatus(context.Background(), "SM-F926N")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Installed || !status.ADBResponsive || status.State != "adb-responsive" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if len(caller.calls) != 3 {
		t.Fatalf("calls = %#v, want package check, broadcast and result read", caller.calls)
	}
	for _, call := range caller.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "monkey") || strings.Contains(joined, "am start") {
			t.Fatalf("status probe must not foreground app: %#v", caller.calls)
		}
	}
	wantPrefix := []string{"-s", "SM-F926N", "shell", "pm", "list", "packages", "com.adbcontrol.companion"}
	if !reflect.DeepEqual(caller.calls[0], wantPrefix) {
		t.Fatalf("package check = %#v, want %#v", caller.calls[0], wantPrefix)
	}
}
