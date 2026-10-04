package device

import (
	"path/filepath"
	"testing"
)

func TestCatalogRemembersDeviceOfflineAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	live := Device{ID: "adb-phone._adb-tls-connect._tcp", Name: "Galaxy Fold", Model: "SM-F926N", State: "device", Transport: "wireless", HardwareID: "serial:R3CR70SJHHR"}
	devices, err := catalog.Merge([]Device{live})
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].State != "device" {
		t.Fatalf("expected one online device, got %#v", devices)
	}

	reopened, err := OpenCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	devices, err = reopened.Merge(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != live.ID || devices[0].State != "offline" {
		t.Fatalf("expected remembered offline device, got %#v", devices)
	}
}

func TestCatalogReconcilesMDNSAliasAndDeletesMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	oldID := "adb-phone (2)._adb-tls-connect._tcp"
	currentID := "adb-phone._adb-tls-connect._tcp"
	if _, err := catalog.Merge([]Device{{ID: oldID, Name: oldID, State: "device", Transport: "wireless"}}); err != nil {
		t.Fatal(err)
	}
	devices, err := catalog.Merge([]Device{{ID: currentID, Name: currentID, State: "device", Transport: "wireless"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != currentID || !containsDeviceID(devices[0].Aliases, oldID) {
		t.Fatalf("expected one reconciled device with old alias, got %#v", devices)
	}

	removed, ok, err := catalog.Remove(oldID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || removed.ID != currentID {
		t.Fatalf("expected alias removal to delete current record, got %#v, %v", removed, ok)
	}
	devices, err = catalog.Merge(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected empty catalog after removal, got %#v", devices)
	}
}

func TestRememberAssignmentsSeedsOfflineCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(nil)
	service.catalog = catalog
	assignedID := "adb-remembered._adb-tls-connect._tcp"
	devices, err := service.RememberAssignments(nil, []string{assignedID})
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != assignedID || devices[0].State != "offline" {
		t.Fatalf("expected assigned device to seed offline catalog, got %#v", devices)
	}
}

func TestCatalogPersistsRemarkAcrossReconnectAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	deviceID := "adb-phone._adb-tls-connect._tcp"
	if _, err := catalog.Merge([]Device{{ID: deviceID, Name: "SM-F926N", State: "device"}}); err != nil {
		t.Fatal(err)
	}
	updated, ok, err := catalog.SetRemark(deviceID, "折叠屏测试机")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || updated.Remark != "折叠屏测试机" {
		t.Fatalf("unexpected updated device: %#v", updated)
	}

	reopened, err := OpenCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	devices, err := reopened.Merge([]Device{{ID: deviceID, Name: "SM-F926N", State: "device"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Remark != "折叠屏测试机" {
		t.Fatalf("remark not retained: %#v", devices)
	}
}
