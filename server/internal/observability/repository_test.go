package observability

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRepositoryStoresQueriesAndRedactsDetails(t *testing.T) {
	// 场景：日志系统必须持久化可查询事件，同时不能把密码/token 等敏感字段写入明文。
	repository, err := OpenRepository(filepath.Join(t.TempDir(), "observability.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service := NewService(repository)

	if err := service.Record(context.Background(), Event{
		Type:      TypeAudit,
		Level:     LevelError,
		Module:    "device.files",
		Action:    "delete_file",
		Message:   "删除文件失败",
		TraceID:   "trace-1",
		ErrorCode: "REMOTE_PATH_INVALID",
		DeviceID:  "device-1",
		Path:      "/api/v1/devices/device-1/files?path=/sdcard/a.txt",
		Details: map[string]any{
			"password": "secret",
			"summary":  "delete /sdcard/a.txt",
		},
	}); err != nil {
		t.Fatal(err)
	}

	events, err := service.List(context.Background(), Query{TraceID: "trace-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Details["password"] != "[redacted]" || events[0].Path != "/api/v1/devices/device-1/files" {
		t.Fatalf("event was not sanitized: %#v", events[0])
	}
	if events[0].ErrorCode != "REMOTE_PATH_INVALID" || events[0].DeviceID != "device-1" {
		t.Fatalf("event lost diagnostic fields: %#v", events[0])
	}

	stats, err := service.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 1 || stats.ErrorCount != 1 || stats.ByType[TypeAudit] != 1 {
		t.Fatalf("bad stats: %#v", stats)
	}
}
