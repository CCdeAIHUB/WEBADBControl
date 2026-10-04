package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
)

type remoteAccessCore struct{}

func (remoteAccessCore) Call(_ context.Context, method string, _ any, target any) error {
	if method != "remote.internal.session.inspect" {
		return nil
	}
	payload, _ := json.Marshal(remoteSession{Username: "operator", Role: "user", Devices: []string{"device-1"}})
	return json.Unmarshal(payload, target)
}

func TestRemoteMediaAuthenticationAcceptsCoreSessionOnlyOnRemoteRoute(t *testing.T) {
	server := &Server{
		devices: device.NewService(remoteAccessCore{}),
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	called := false
	handler := server.authentication(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/remote/devices/device-1/screen", nil)
	request.Header.Set("Authorization", "Bearer remote-session")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !called {
		t.Fatalf("remote media request was not authorized: status=%d", response.Code)
	}

	called = false
	request = httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	request.Header.Set("Authorization", "Bearer remote-session")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if called || response.Code != http.StatusUnauthorized {
		t.Fatalf("remote session escaped into administrator API: called=%v status=%d", called, response.Code)
	}
}

func TestRemoteSessionChecksAssignedDevice(t *testing.T) {
	session := remoteSession{Devices: []string{"device-1"}}
	if !session.canAccessDevice("device-1") || session.canAccessDevice("device-2") {
		t.Fatal("assigned-device boundary is incorrect")
	}
}

func TestRemoteSessionAcceptsOnlyEquivalentMDNSCollisionAlias(t *testing.T) {
	// 场景：开启 tcpip 导致 ADB 重建连接后，mDNS 的本机冲突序号可能消失；
	// 远程媒体应继续访问同一服务名，但不能因此越权到其他设备。
	session := remoteSession{Devices: []string{"adb-R3CR70SJHHR-2VyQ5v (2)._adb-tls-connect._tcp"}}
	if !session.canAccessDevice("adb-R3CR70SJHHR-2VyQ5v._adb-tls-connect._tcp") {
		t.Fatal("equivalent mDNS alias was rejected")
	}
	if session.canAccessDevice("adb-other._adb-tls-connect._tcp") {
		t.Fatal("unrelated mDNS device was authorized")
	}
}

func TestRemovedDeviceMatchesPersistedMDNSAssignmentAlias(t *testing.T) {
	assigned := "adb-R3CR70SJHHR-2VyQ5v (2)._adb-tls-connect._tcp"
	removed := []string{"adb-R3CR70SJHHR-2VyQ5v._adb-tls-connect._tcp"}
	if !matchesRemovedDevice(assigned, removed) {
		t.Fatal("expected removed current identity to match persisted collision alias")
	}
	if matchesRemovedDevice("adb-other._adb-tls-connect._tcp", removed) {
		t.Fatal("unrelated assignment matched removed device")
	}
}
