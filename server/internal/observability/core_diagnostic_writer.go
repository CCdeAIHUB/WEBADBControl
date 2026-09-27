package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
)

// CoreDiagnosticWriter imports the metadata-only mobile diagnostics emitted by
// Core stderr into the same repository used by the Web log page. Other stderr
// lines remain available through the container log and are intentionally ignored.
type CoreDiagnosticWriter struct {
	mu      sync.Mutex
	buffer  []byte
	service *Service
	logger  *slog.Logger
}

func NewCoreDiagnosticWriter(service *Service, logger *slog.Logger) *CoreDiagnosticWriter {
	return &CoreDiagnosticWriter{service: service, logger: logger}
}

func (w *CoreDiagnosticWriter) Write(chunk []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buffer = append(w.buffer, chunk...)
	for {
		index := bytes.IndexByte(w.buffer, '\n')
		if index < 0 {
			break
		}
		line := append([]byte(nil), w.buffer[:index]...)
		w.buffer = w.buffer[index+1:]
		w.importLine(line)
	}
	if len(w.buffer) > 64*1024 {
		w.buffer = nil
	}
	return len(chunk), nil
}

func (w *CoreDiagnosticWriter) importLine(line []byte) {
	var payload struct {
		Event     string `json:"event"`
		Actor     string `json:"actor"`
		SessionID string `json:"sessionId"`
		Level     string `json:"level"`
		Phase     string `json:"phase"`
		Module    string `json:"module"`
		OK        bool   `json:"ok"`
		ElapsedMS int64  `json:"elapsedMs"`
		ErrorCode string `json:"errorCode"`
		Detail    string `json:"detail"`
	}
	if json.Unmarshal(line, &payload) != nil || payload.Event != "mobile_client_diagnostic" {
		return
	}
	level := payload.Level
	if level != LevelInfo && level != LevelWarn && level != LevelError {
		level = LevelInfo
	}
	err := w.service.Record(context.Background(), Event{
		Type:       TypeClientError,
		Level:      level,
		Module:     "mobile." + payload.Module,
		Action:     payload.Phase,
		Message:    "移动端诊断：" + payload.Phase,
		TraceID:    "mobile-" + payload.SessionID,
		ErrorCode:  payload.ErrorCode,
		DurationMS: payload.ElapsedMS,
		Actor:      payload.Actor,
		Details: map[string]any{
			"outcome": payload.OK,
			"detail":  payload.Detail,
		},
	})
	if err != nil && w.logger != nil {
		w.logger.Warn("mobile_diagnostic_import_failed", "error", err)
	}
}
