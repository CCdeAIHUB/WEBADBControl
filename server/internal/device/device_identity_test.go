package device

import (
	"context"
	"encoding/json"
	"testing"
)

type identityCore struct{ output string }

func (core identityCore) Call(_ context.Context, method string, _ any, target any) error {
	if method != "adb.exec" {
		return nil
	}
	payload, _ := json.Marshal(CommandOutput{Stdout: core.output})
	return json.Unmarshal(payload, target)
}

func TestResolveOnlineDeviceIDMigratesMDNSCollisionSuffix(t *testing.T) {
	service := NewService(identityCore{output: "List of devices attached\nadb-R3CR70SJHHR-2VyQ5v._adb-tls-connect._tcp device model:SM_F926N\n"})
	resolved, err := service.ResolveOnlineDeviceID(context.Background(), "adb-R3CR70SJHHR-2VyQ5v (2)._adb-tls-connect._tcp")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "adb-R3CR70SJHHR-2VyQ5v._adb-tls-connect._tcp" {
		t.Fatalf("resolved id = %q", resolved)
	}
}

func TestEquivalentDeviceIDRejectsDifferentService(t *testing.T) {
	if EquivalentDeviceID("adb-a (2)._adb-tls-connect._tcp", "adb-b._adb-tls-connect._tcp") {
		t.Fatal("different mDNS services were considered equivalent")
	}
}
