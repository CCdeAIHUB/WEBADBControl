package device

import "testing"

func TestParseHardwareIDPrefersStableDeviceValues(t *testing.T) {
	// 场景：同一手机以 USB 和无线同时出现时，服务端必须使用设备硬件 ID 识别为同一台设备。
	id := ParseHardwareID("serial=unknown\nbootserial=R58M123456\nandroid_id=abc\n")
	if id != "bootserial:R58M123456" {
		t.Fatalf("hardware id = %q", id)
	}
}

func TestDedupeDevicesByHardwareIDPrefersUSBTransport(t *testing.T) {
	// 场景：同一硬件 ID 同时存在 USB 和无线 transport，设备中心只能显示一个 card。
	devices := dedupeDevicesByHardwareID([]Device{
		{ID: "192.168.3.20:5555", State: "device", Transport: "wireless", HardwareID: "serial:R58M123456"},
		{ID: "R58M123456", State: "device", Transport: "usb", HardwareID: "serial:R58M123456"},
	})
	if len(devices) != 1 {
		t.Fatalf("device count = %d, want 1", len(devices))
	}
	if devices[0].ID != "R58M123456" || len(devices[0].Aliases) != 1 || devices[0].Aliases[0] != "192.168.3.20:5555" {
		t.Fatalf("bad deduped device: %#v", devices[0])
	}
}
