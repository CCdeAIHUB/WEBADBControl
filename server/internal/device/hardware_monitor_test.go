package device

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestHardwareMonitorRunsSeriallyUntilExplicitStop(t *testing.T) {
	// 场景：采样耗时超过间隔时不得重入；页面消失不会取消任务，只有 Stop 才停止。
	var active atomic.Int32
	var maximum atomic.Int32
	collector := func(context.Context, string) (HardwareSnapshot, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			seen := maximum.Load()
			if current <= seen || maximum.CompareAndSwap(seen, current) {
				break
			}
		}
		time.Sleep(35 * time.Millisecond)
		return HardwareSnapshot{CapturedAt: time.Now().UTC()}, nil
	}
	manager := NewHardwareMonitorManager(collector, 20*time.Millisecond, 20)
	manager.Start("phone", []string{"cpu", "memory"})
	time.Sleep(250 * time.Millisecond)
	status := manager.Status("phone")
	if !status.Running || status.SampleCount < 2 {
		t.Fatalf("monitor did not remain active: %#v", status)
	}
	if maximum.Load() != 1 {
		t.Fatalf("collector overlapped: max concurrency %d", maximum.Load())
	}
	manager.Stop("phone")
	stoppedAt := manager.Status("phone").SampleCount
	time.Sleep(70 * time.Millisecond)
	if status := manager.Status("phone"); status.Running || status.SampleCount != stoppedAt {
		t.Fatalf("explicit stop was not terminal: %#v", status)
	}
}

func TestHardwareMonitorRestoresRunningTasksAfterServiceRestart(t *testing.T) {
	// 场景：服务重启不是手动停止；持久化为 running 的记录任务必须自动恢复。
	path := filepath.Join(t.TempDir(), "monitor.json")
	collector := func(context.Context, string) (HardwareSnapshot, error) {
		return HardwareSnapshot{CapturedAt: time.Now().UTC()}, nil
	}
	first := NewHardwareMonitorManager(collector, 25*time.Millisecond, 10)
	if err := first.EnablePersistence(path); err != nil {
		t.Fatal(err)
	}
	first.Start("phone", []string{"cpu"})
	second := NewHardwareMonitorManager(collector, 25*time.Millisecond, 10)
	if err := second.EnablePersistence(path); err != nil {
		t.Fatal(err)
	}
	defer first.Stop("phone")
	defer second.Stop("phone")
	if status := second.Status("phone"); !status.Running {
		t.Fatalf("running task was not restored: %#v", status)
	}
}

func TestHardwareMonitorStartIsIdempotent(t *testing.T) {
	// 场景：重复进入页面并再次查询/启动不得创建第二个后台采样循环。
	manager := NewHardwareMonitorManager(func(context.Context, string) (HardwareSnapshot, error) {
		return HardwareSnapshot{CapturedAt: time.Now().UTC()}, nil
	}, 25*time.Millisecond, 10)
	first := manager.Start("phone", nil)
	second := manager.Start("phone", nil)
	defer manager.Stop("phone")
	if first.StartedAt.IsZero() || !first.StartedAt.Equal(second.StartedAt) {
		t.Fatalf("duplicate start replaced the active task: first=%v second=%v", first.StartedAt, second.StartedAt)
	}
}

func TestHardwareMonitorReportsPersistenceFailure(t *testing.T) {
	// 场景：持久化目录不可创建时，后台任务仍可运行，但错误必须通过状态接口可见。
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewHardwareMonitorManager(func(context.Context, string) (HardwareSnapshot, error) {
		return HardwareSnapshot{CapturedAt: time.Now().UTC()}, nil
	}, 25*time.Millisecond, 10)
	manager.statePath = filepath.Join(parentFile, "monitor.json")
	manager.Start("phone", nil)
	defer manager.Stop("phone")
	if status := manager.Status("phone"); status.LastErrorCode != "HARDWARE_MONITOR_STATE_PERSIST_FAILED" {
		t.Fatalf("persistence failure was hidden: %#v", status)
	}
}
