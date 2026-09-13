package device

import (
	"encoding/binary"
	"testing"
)

func TestEncodeScrcpyTouchPreservesMobilePointerAndVideoSpace(t *testing.T) {
	// 场景：手机浏览器在预览画面按下时，控制包必须使用视频坐标空间和非零压力，不能退化为延迟的 adb tap。
	message, err := EncodeScrcpyTouch(0, 7, 320, 640, 1080, 2400)
	if err != nil {
		t.Fatal(err)
	}
	if len(message) != 32 || message[0] != 2 || message[1] != 0 || binary.BigEndian.Uint32(message[10:14]) != 320 || binary.BigEndian.Uint32(message[14:18]) != 640 || binary.BigEndian.Uint16(message[18:20]) != 1080 || binary.BigEndian.Uint16(message[22:24]) != 65535 {
		t.Fatalf("bad touch message: %x", message)
	}
}

func TestEncodeScrcpyTouchRejectsCoordinatesOutsideVideo(t *testing.T) {
	// 场景：旋转或缩放后的陈旧坐标不得发往设备，避免触控落到错误位置。
	if _, err := EncodeScrcpyTouch(2, 1, 1080, 1, 1080, 2400); err == nil {
		t.Fatal("out-of-range touch was accepted")
	}
}

func TestNormalizeScrcpyOptionsUsesVideoDefaultsAndBounds(t *testing.T) {
	defaults := normalizeScrcpyOptions(ScrcpyOptions{})
	if defaults.MaxSize != 1280 || defaults.BitRate != 4_000_000 || defaults.FrameRate != 30 {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}

	selected := normalizeScrcpyOptions(ScrcpyOptions{MaxSize: 1920, BitRate: 8_000_000, FrameRate: 60})
	if selected.MaxSize != 1920 || selected.BitRate != 8_000_000 || selected.FrameRate != 60 {
		t.Fatalf("valid video options were changed: %#v", selected)
	}
}

func TestRandomScrcpyIDAlwaysFitsJavaSignedInteger(t *testing.T) {
	for range 256 {
		value, err := randomScrcpyID()
		if err != nil {
			t.Fatal(err)
		}
		if value < 0x10000000 || value > 0x7fffffff {
			t.Fatalf("scrcpy id is outside Java signed integer range: %08x", value)
		}
	}
}
