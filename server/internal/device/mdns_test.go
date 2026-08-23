package device

import "testing"

func TestParseMDNSSupportsIPv4IPv6AndDeduplicates(t *testing.T) {
	// 场景：现代 ADB mDNS 可能同时返回 IPv4/IPv6；重复广播不能在连接窗口重复展示。
	raw := "adb-one _adb-tls-pairing._tcp 192.168.3.20:37123\n" +
		"adb-two _adb-tls-connect._tcp [fe80::1234]:5555\n" +
		"adb-one _adb-tls-pairing._tcp 192.168.3.20:37123\n"

	services := ParseMDNS(raw)

	if len(services) != 2 {
		t.Fatalf("services = %#v", services)
	}
	if services[0].Type != "pairing" || services[0].Endpoint != "192.168.3.20:37123" {
		t.Fatalf("unexpected IPv4 pairing service: %#v", services[0])
	}
	if services[1].Type != "connect" || services[1].Endpoint != "[fe80::1234]:5555" {
		t.Fatalf("unexpected IPv6 connect service: %#v", services[1])
	}
}
