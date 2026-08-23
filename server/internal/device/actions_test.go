package device

import (
	"reflect"
	"testing"
)

func TestActionArgumentsAreWhitelistedAndAddressed(t *testing.T) {
	// 场景：网页设备操作必须映射到固定 ADB 参数，设备 ID 由服务端插入。
	args, err := ActionArguments("serial-1", ActionRequest{Type: "key", Key: "HOME"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-s", "serial-1", "shell", "input", "keyevent", "KEYCODE_HOME"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("got %#v want %#v", args, want)
	}
}

func TestActionArgumentsMapSemanticKeysToAndroidKeyCodes(t *testing.T) {
	// 场景：网页发送语义按键，服务端必须映射为 Android input keyevent 稳定识别的 KEYCODE_*。
	args, err := ActionArguments("serial-1", ActionRequest{Type: "key", Key: "APP_SWITCH"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-s", "serial-1", "shell", "input", "keyevent", "KEYCODE_APP_SWITCH"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("got %#v want %#v", args, want)
	}
}

func TestActionArgumentsRejectUnknownOrUnsafeValues(t *testing.T) {
	// 场景：未知动作和包含 NUL 的文本必须被协议层拒绝，不能进入 Core。
	if _, err := ActionArguments("serial-1", ActionRequest{Type: "host-shell", Text: "rm"}); err == nil {
		t.Fatal("expected unknown action to fail")
	}
	if _, err := ActionArguments("serial-1", ActionRequest{Type: "text", Text: "bad\x00value"}); err == nil {
		t.Fatal("expected NUL text to fail")
	}
}

func TestTapAndSwipeValidateCoordinates(t *testing.T) {
	// 场景：触控坐标必须非负且完整，避免无效坐标进入设备控制通道。
	if _, err := ActionArguments("serial-1", ActionRequest{Type: "tap", X: -1, Y: 20}); err == nil {
		t.Fatal("expected negative coordinate to fail")
	}
	if _, err := ActionArguments("serial-1", ActionRequest{Type: "swipe", X: 1, Y: 2, EndX: 3, EndY: 4, DurationMS: 250}); err != nil {
		t.Fatal(err)
	}
}
