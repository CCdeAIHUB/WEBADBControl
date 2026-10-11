package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
)

type fileTransfer struct {
	ID               string     `json:"id"`
	DeviceID         string     `json:"deviceId"`
	RemotePath       string     `json:"remotePath"`
	FileName         string     `json:"fileName"`
	State            string     `json:"state"`
	BytesTotal       int64      `json:"bytesTotal"`
	BytesTransferred int64      `json:"bytesTransferred"`
	Progress         float64    `json:"progress"`
	Transport        string     `json:"transport"`
	Error            string     `json:"error,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
	localPath        string
	cancel           context.CancelFunc
}

type fileTransferManager struct {
	mu      sync.Mutex
	jobs    map[string]*fileTransfer
	dataDir string
	devices *device.Service
	logger  *slog.Logger
}

func newFileTransferManager(dataDir string, devices *device.Service, logger *slog.Logger) *fileTransferManager {
	directory := filepath.Join(dataDir, "downloads")
	_ = os.MkdirAll(directory, 0o750)
	return &fileTransferManager{jobs: map[string]*fileTransfer{}, dataDir: directory, devices: devices, logger: logger}
}

func (m *fileTransferManager) start(deviceID, remotePath string) (*fileTransfer, error) {
	if err := device.ValidateRemotePath(remotePath); err != nil {
		return nil, err
	}
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	localPath := filepath.Join(m.dataDir, "download-"+id)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	job := &fileTransfer{ID: id, DeviceID: deviceID, RemotePath: remotePath, FileName: filepath.Base(remotePath), State: "queued", Transport: "adb-isolated", CreatedAt: time.Now().UTC(), localPath: localPath, cancel: cancel}
	m.mu.Lock()
	m.jobs[id] = job
	m.mu.Unlock()
	go m.run(ctx, job)
	return m.snapshot(job), nil
}

func (m *fileTransferManager) run(ctx context.Context, job *fileTransfer) {
	m.update(job.ID, func(current *fileTransfer) { current.State = "running" })
	if output, err := m.devices.ExecTransfer(ctx, device.DeviceArgs(job.DeviceID, "shell", "stat", "-c", "%s", job.RemotePath)); err == nil {
		var total int64
		_, _ = fmt.Sscan(output.Stdout, &total)
		m.update(job.ID, func(current *fileTransfer) { current.BytesTotal = total })
	}
	_, err := m.devices.ExecTransfer(ctx, device.DeviceArgs(job.DeviceID, "pull", job.RemotePath, job.localPath))
	now := time.Now().UTC()
	m.update(job.ID, func(current *fileTransfer) {
		current.CompletedAt = &now
		if err != nil {
			current.State = "failed"
			current.Error = err.Error()
		} else {
			current.State = "completed"
			if info, statErr := os.Stat(current.localPath); statErr == nil {
				current.BytesTransferred = info.Size()
				if current.BytesTotal == 0 {
					current.BytesTotal = info.Size()
				}
			}
			current.Progress = 100
		}
	})
	if err != nil && m.logger != nil {
		m.logger.Warn("file_transfer_failed", "deviceId", job.DeviceID, "path", job.RemotePath, "error", err)
	}
	time.AfterFunc(30*time.Minute, func() { m.remove(job.ID) })
}

func (m *fileTransferManager) update(id string, fn func(*fileTransfer)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job := m.jobs[id]; job != nil {
		fn(job)
	}
}

func (m *fileTransferManager) snapshot(job *fileTransfer) *fileTransfer {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current := m.jobs[job.ID]; current != nil {
		if info, err := os.Stat(current.localPath); err == nil && current.State == "running" {
			current.BytesTransferred = info.Size()
		}
		if current.BytesTotal > 0 {
			current.Progress = float64(current.BytesTransferred) * 100 / float64(current.BytesTotal)
		}
		copy := *current
		copy.localPath = ""
		copy.cancel = nil
		return &copy
	}
	return nil
}

func (m *fileTransferManager) get(id, deviceID string) (*fileTransfer, bool) {
	m.mu.Lock()
	job := m.jobs[id]
	m.mu.Unlock()
	if job == nil || job.DeviceID != deviceID {
		return nil, false
	}
	return m.snapshot(job), true
}

func (m *fileTransferManager) localPath(id, deviceID string) (string, *fileTransfer, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.jobs[id]
	if job == nil || job.DeviceID != deviceID || job.State != "completed" {
		return "", nil, false
	}
	copy := *job
	copy.localPath = ""
	copy.cancel = nil
	return job.localPath, &copy, true
}

func (m *fileTransferManager) remove(id string) {
	m.mu.Lock()
	job := m.jobs[id]
	delete(m.jobs, id)
	m.mu.Unlock()
	if job != nil {
		job.cancel()
		_ = os.Remove(job.localPath)
	}
}

func (s *Server) startFileDownload(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	job, err := s.transfers.start(request.PathValue("id"), body.Path)
	if err != nil {
		writeError(writer, http.StatusBadRequest, apperror.Wrap("REMOTE_PATH_INVALID", "设备路径无效", "device.files", true, err))
		return
	}
	writeData(writer, http.StatusAccepted, job)
}

func (s *Server) fileDownloadStatus(writer http.ResponseWriter, request *http.Request) {
	job, ok := s.transfers.get(request.PathValue("transferId"), request.PathValue("id"))
	if !ok {
		writeError(writer, http.StatusNotFound, apperror.New("FILE_TRANSFER_NOT_FOUND", "下载任务不存在或已过期", "device.files", false))
		return
	}
	writeData(writer, http.StatusOK, job)
}

func (s *Server) fileDownloadContent(writer http.ResponseWriter, request *http.Request) {
	path, job, ok := s.transfers.localPath(request.PathValue("transferId"), request.PathValue("id"))
	if !ok {
		writeError(writer, http.StatusConflict, apperror.New("FILE_TRANSFER_NOT_READY", "文件尚未传输完成", "device.files", true))
		return
	}
	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", job.FileName))
	http.ServeFile(writer, request, path)
}

func (s *Server) cancelFileDownload(writer http.ResponseWriter, request *http.Request) {
	s.transfers.remove(request.PathValue("transferId"))
	writeData(writer, http.StatusOK, map[string]bool{"cancelled": true})
}
