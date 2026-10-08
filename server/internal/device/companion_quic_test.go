package device

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type companionProvisionCaller struct {
	calls [][]string
}

func (c *companionProvisionCaller) Call(_ context.Context, method string, params any, target any) error {
	switch method {
	case "companion.connection.info":
		*(target.(*CompanionConnectionInfo)) = CompanionConnectionInfo{
			ListenAddress:                "0.0.0.0:45922",
			ServerName:                   "adbcontrol-core",
			CertificateDERBase64:         "AQIDBA==",
			CertificateFingerprintSHA256: strings.Repeat("a", 64),
		}
		return nil
	case "adb.exec":
		args := params.(map[string]any)["args"].([]string)
		c.calls = append(c.calls, append([]string(nil), args...))
		*(target.(*CommandOutput)) = CommandOutput{ExitCode: 0, Stdout: "Broadcast completed: result=0"}
		return nil
	default:
		return fmt.Errorf("unexpected method %q", method)
	}
}

func TestProvisionCompanionQUICPushesPinnedCoreConfiguration(t *testing.T) {
	// 场景：伴侣已安装但 QUIC 未连接时，Web 必须通过显式组件广播下发可达端点、TLS 名称和固定证书。
	caller := &companionProvisionCaller{}
	service := NewService(caller, WithCompanionPublicHost("192.168.3.1"))

	result, err := service.ProvisionCompanionQUIC(context.Background(), "192.168.3.168:39883")
	if err != nil {
		t.Fatal(err)
	}
	if result.Endpoint != "quic://192.168.3.1:45922" || result.ServerName != "adbcontrol-core" {
		t.Fatalf("unexpected provisioning result: %#v", result)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("ADB calls = %d, want 1", len(caller.calls))
	}
	wantPrefix := []string{"-s", "192.168.3.168:39883", "shell"}
	if !reflect.DeepEqual(caller.calls[0][:3], wantPrefix) {
		t.Fatalf("ADB prefix = %#v, want %#v", caller.calls[0][:3], wantPrefix)
	}
	command := caller.calls[0][3]
	for _, required := range []string{
		"com.adbcontrol.companion.CONFIGURE_CONNECTION",
		"com.adbcontrol.companion/.quic.CompanionConnectionReceiver",
		"quic://192.168.3.1:45922",
		"--es deviceId '192.168.3.168:39883'",
		"--es serverName 'adbcontrol-core'",
		"--es certificateDerBase64 'AQIDBA=='",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("command missing %q: %s", required, command)
		}
	}
}

func TestResolveCompanionEndpointUsesRouteAddressForWirelessDevice(t *testing.T) {
	// 场景：未显式配置局域网地址时，应根据到无线设备的路由选择 Core 地址，不能下发 0.0.0.0。
	host, err := resolveCompanionPublicHost("", "192.168.3.168:39883", func(_ string) (string, error) {
		return "192.168.3.1", nil
	})
	if err != nil || host != "192.168.3.1" {
		t.Fatalf("host=%q err=%v", host, err)
	}
}

func TestResolveCompanionEndpointRejectsUnroutableListenAddress(t *testing.T) {
	// 场景：既没有显式地址也无法由设备路由推导时，流程必须返回结构化错误，不能把 0.0.0.0 发给手机。
	_, err := resolveCompanionPublicHost("", "usb-device", func(_ string) (string, error) {
		return "", context.DeadlineExceeded
	})
	if err == nil || !strings.Contains(err.Error(), "COMPANION_QUIC_PUBLIC_HOST_UNAVAILABLE") {
		t.Fatalf("error=%v", err)
	}
}
