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

func TestParseDevicesKeepsMDNSInstanceSuffixInWirelessSerial(t *testing.T) {
	// 场景：ADB 为重复 mDNS 服务自动追加“(2)”时，空格仍属于序列号，不能把它拆成 USB 设备和错误状态。
	devices := parseDevices("List of devices attached\nadb-16a424c7-X9sGA6 (2)._adb-tls-connect._tcp device product:onyx model:25053RT47C\n")

	if len(devices) != 1 {
		t.Fatalf("device count = %d, want 1", len(devices))
	}
	if devices[0].ID != "adb-16a424c7-X9sGA6 (2)._adb-tls-connect._tcp" || devices[0].State != "device" || devices[0].Transport != "wireless" {
		t.Fatalf("parsed device = %#v", devices[0])
	}
}

func TestDedupeDevicesByNormalizedMDNSServiceIdentity(t *testing.T) {
	// 场景：同一无线服务的重复实例仅差 ADB 添加的“(n)”后缀，硬件属性暂不可读时也只能显示一张卡片。
	devices := dedupeDevicesByHardwareID([]Device{
		{ID: "adb-16a424c7-X9sGA6._adb-tls-connect._tcp", State: "device", Transport: "wireless", HardwareID: fallbackHardwareID(Device{ID: "adb-16a424c7-X9sGA6._adb-tls-connect._tcp", Transport: "wireless"})},
		{ID: "adb-16a424c7-X9sGA6 (2)._adb-tls-connect._tcp", State: "device", Transport: "wireless", HardwareID: fallbackHardwareID(Device{ID: "adb-16a424c7-X9sGA6 (2)._adb-tls-connect._tcp", Transport: "wireless"})},
	})

	if len(devices) != 1 || len(devices[0].Aliases) != 1 || devices[0].Aliases[0] != "adb-16a424c7-X9sGA6 (2)._adb-tls-connect._tcp" {
		t.Fatalf("deduped devices = %#v", devices)
	}
}
