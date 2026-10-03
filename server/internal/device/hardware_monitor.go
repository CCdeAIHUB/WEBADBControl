package device

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const defaultHardwareMonitorHistory = 2400

type HardwareMonitorStatus struct {
	DeviceID       string             `json:"deviceId"`
	Running        bool               `json:"running"`
	StartedAt      time.Time          `json:"startedAt,omitempty"`
	IntervalMillis int64              `json:"intervalMillis"`
	Metrics        []string           `json:"metrics"`
	SampleCount    int                `json:"sampleCount"`
	Samples        []HardwareSnapshot `json:"samples"`
	LastError      string             `json:"lastError,omitempty"`
	LastErrorCode  string             `json:"lastErrorCode,omitempty"`
}

type hardwareCollector func(context.Context, string) (HardwareSnapshot, error)

type hardwareMonitorTask struct {
	mu        sync.RWMutex
	status    HardwareMonitorStatus
	cancel    context.CancelFunc
	collector hardwareCollector
	interval  time.Duration
	limit     int
	statePath string
}

type HardwareMonitorManager struct {
	mu         sync.RWMutex
	tasks      map[string]*hardwareMonitorTask
	collector  hardwareCollector
	interval   time.Duration
	limit      int
	statePath  string
	persistErr string
}

func NewHardwareMonitorManager(collector hardwareCollector, interval time.Duration, limit int) *HardwareMonitorManager {
	if interval < 20*time.Millisecond {
		interval = 3 * time.Second
	}
	if limit < 1 {
		limit = defaultHardwareMonitorHistory
	}
	return &HardwareMonitorManager{tasks: map[string]*hardwareMonitorTask{}, collector: collector, interval: interval, limit: limit}
}

func (m *HardwareMonitorManager) Start(deviceID string, metrics []string) HardwareMonitorStatus {
	m.mu.Lock()
	if current := m.tasks[deviceID]; current != nil {
		m.mu.Unlock()
		return current.snapshot()
	}
	ctx, cancel := context.WithCancel(context.Background())
	task := &hardwareMonitorTask{
		status: HardwareMonitorStatus{DeviceID: deviceID, Running: true, StartedAt: time.Now().UTC(), IntervalMillis: m.interval.Milliseconds(), Metrics: append([]string(nil), metrics...)},
		cancel: cancel, collector: m.collector, interval: m.interval, limit: m.limit,
	}
	m.tasks[deviceID] = task
	m.mu.Unlock()
	go task.run(ctx)
	m.recordPersistenceError(m.persist())
	return task.snapshot()
}

func (m *HardwareMonitorManager) Stop(deviceID string) HardwareMonitorStatus {
	m.mu.RLock()
	task := m.tasks[deviceID]
	m.mu.RUnlock()
	if task == nil {
		return HardwareMonitorStatus{DeviceID: deviceID, IntervalMillis: m.interval.Milliseconds()}
	}
	task.mu.Lock()
	if task.status.Running {
		task.status.Running = false
		task.cancel()
	}
	task.mu.Unlock()
	m.recordPersistenceError(m.persist())
	return task.snapshot()
}

func (m *HardwareMonitorManager) EnablePersistence(path string) error {
	m.mu.Lock()
	m.statePath = path
	m.mu.Unlock()
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var saved []HardwareMonitorStatus
	if err := json.Unmarshal(content, &saved); err != nil {
		return err
	}
	for _, status := range saved {
		if status.Running && status.DeviceID != "" {
			m.Start(status.DeviceID, status.Metrics)
		}
	}
	return nil
}

func (m *HardwareMonitorManager) persist() error {
	m.mu.RLock()
	path := m.statePath
	tasks := make([]HardwareMonitorStatus, 0, len(m.tasks))
	for _, task := range m.tasks {
		status := task.snapshot()
		if status.Running {
			status.Samples = nil
			tasks = append(tasks, status)
		}
	}
	m.mu.RUnlock()
	if path == "" {
		return nil
	}
	content, err := json.Marshal(tasks)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func (m *HardwareMonitorManager) recordPersistenceError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		m.persistErr = ""
		return
	}
	m.persistErr = err.Error()
}

func (m *HardwareMonitorManager) Status(deviceID string) HardwareMonitorStatus {
	m.mu.RLock()
	task := m.tasks[deviceID]
	m.mu.RUnlock()
	if task == nil {
		status := HardwareMonitorStatus{DeviceID: deviceID, IntervalMillis: m.interval.Milliseconds(), Samples: []HardwareSnapshot{}}
		m.applyPersistenceError(&status)
		return status
	}
	status := task.snapshot()
	m.applyPersistenceError(&status)
	return status
}

func (m *HardwareMonitorManager) applyPersistenceError(status *HardwareMonitorStatus) {
	m.mu.RLock()
	persistErr := m.persistErr
	m.mu.RUnlock()
	if persistErr != "" {
		status.LastError = persistErr
		status.LastErrorCode = "HARDWARE_MONITOR_STATE_PERSIST_FAILED"
	}
}

func (t *hardwareMonitorTask) run(ctx context.Context) {
	for {
		started := time.Now()
		sample, err := t.collector(ctx, t.status.DeviceID)
		t.mu.Lock()
		if err != nil {
			t.status.LastError = err.Error()
			t.status.LastErrorCode = "HARDWARE_MONITOR_SAMPLE_FAILED"
		} else {
			t.status.LastError, t.status.LastErrorCode = "", ""
			t.status.Samples = append(t.status.Samples, sample)
			if len(t.status.Samples) > t.limit {
				t.status.Samples = append([]HardwareSnapshot(nil), t.status.Samples[len(t.status.Samples)-t.limit:]...)
			}
			t.status.SampleCount++
		}
		running := t.status.Running
		t.mu.Unlock()
		if !running {
			return
		}
		// The next delay starts after collection completes. Slow ADB never creates a queue.
		delay := t.interval - time.Since(started)
		if delay < 100*time.Millisecond {
			delay = 100 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (t *hardwareMonitorTask) snapshot() HardwareMonitorStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := t.status
	result.Metrics = append([]string(nil), t.status.Metrics...)
	result.Samples = append([]HardwareSnapshot(nil), t.status.Samples...)
	return result
}
