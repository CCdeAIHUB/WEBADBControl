package device

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

type companionStatusCaller struct {
	calls [][]string
}

type companionFallbackCaller struct {
	catAttempts int
}

func (c *companionFallbackCaller) Call(_ context.Context, method string, params any, target any) error {
	switch method {
	case "device.getCapabilities", "device.getPermissionState", "device.invoke":
		return apperror.New("COMPANION_DEVICE_NOT_CONNECTED", "QUIC session is not connected", "companion.registry", true)
	case "capability.list":
		*(target.(*[]map[string]any)) = []map[string]any{{"id": "android.accessibility.control"}}
		return nil
	case "adb.exec":
		args := params.(map[string]any)["args"].([]string)
		output := CommandOutput{}
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "cat /sdcard/Android/data/com.adbcontrol.companion/files/command-results/") {
			c.catAttempts++
			if c.catAttempts < 10 {
				output.ExitCode = 1
				output.Stderr = "result is not ready"
			} else {
				output.Stdout = `{"requestId":"test","ok":true,"result":{"enabled":false}}`
			}
		}
		*(target.(*CommandOutput)) = output
		return nil
	default:
		return errors.New("unexpected method: " + method)
	}
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
	case strings.Contains(joined, "dumpsys package com.adbcontrol.companion"):
		output.Stdout = "Packages:\n  Package [com.adbcontrol.companion]:\n    versionCode=13 minSdk=29 targetSdk=36\n    versionName=0.13.0\n"
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
	wantPrefix := []string{"-s", "SM-F926N", "shell", "dumpsys", "package", "com.adbcontrol.companion"}
	if !reflect.DeepEqual(caller.calls[0], wantPrefix) {
		t.Fatalf("package check = %#v, want %#v", caller.calls[0], wantPrefix)
	}
}

func TestCompanionCapabilitiesFallBackToCoreCatalogWithoutQuic(t *testing.T) {
	// 场景：Companion 的 QUIC 会话尚未建立时，Web 仍应展示 Core 能力目录，不能把可用的 ADB 通道误报为完全断开。
	access, err := NewService(&companionFallbackCaller{}).CompanionCapabilities(context.Background(), "wireless-device")
	if err != nil {
		t.Fatal(err)
	}
	if access.Transport != CompanionTransportADBBroadcast || len(access.Items) != 1 {
		t.Fatalf("unexpected access: %#v", access)
	}
}

func TestCompanionPermissionsReturnExplicitADBTransportWithoutQuic(t *testing.T) {
	// 场景：ADB 兼容通道无法主动同步完整权限矩阵时，应返回显式 transport 和空集合，不能返回 502。
	access, err := NewService(&companionFallbackCaller{}).CompanionPermissions(context.Background(), "wireless-device")
	if err != nil {
		t.Fatal(err)
	}
	if access.Transport != CompanionTransportADBBroadcast || len(access.Items) != 0 {
		t.Fatalf("unexpected access: %#v", access)
	}
}

func TestCompanionInvokeWaitsForColdStartedBroadcastResult(t *testing.T) {
	// 场景：无线设备冷启动 Companion 超过旧的 800ms 轮询窗时，仍需等到结果文件出现并通过 ADB 完成调用。
	caller := &companionFallbackCaller{}
	result, transport, err := NewService(caller).InvokeCompanionCapability(
		context.Background(),
		"wireless-device",
		"android.accessibility.control",
		"accessibility.status",
		map[string]any{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if transport != CompanionTransportADBBroadcast || caller.catAttempts != 10 || result["ok"] != true {
		t.Fatalf("result=%#v transport=%q attempts=%d", result, transport, caller.catAttempts)
	}
}
