package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

const (
	TypeRequest     = "request"
	TypeAudit       = "audit"
	TypeClientError = "client_error"
	TypeSystem      = "system"

	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Record(ctx context.Context, event Event) error {
	if s == nil || s.repository == nil {
		return nil
	}
	event = normalizeEvent(event)
	return s.repository.Save(ctx, event)
}

func (s *Service) List(ctx context.Context, query Query) ([]Event, error) {
	if s == nil || s.repository == nil {
		return []Event{}, nil
	}
	return s.repository.List(ctx, query)
}

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	if s == nil || s.repository == nil {
		return Stats{ByType: map[string]int64{}, ByLevel: map[string]int64{}}, nil
	}
	return s.repository.Stats(ctx)
}

func normalizeEvent(event Event) Event {
	if event.ID == "" {
		event.ID = newID()
	}
	if event.Type == "" {
		event.Type = TypeSystem
	}
	if event.Level == "" {
		event.Level = LevelInfo
	}
	if event.Module == "" {
		event.Module = "system"
	}
	if event.Action == "" {
		event.Action = event.Type
	}
	if event.Message == "" {
		event.Message = event.Action
	}
	if event.TraceID == "" {
		event.TraceID = "trace-unavailable"
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	event.Message = truncate(event.Message, 1000)
	event.Path = sanitizePath(event.Path)
	event.UserAgent = truncate(event.UserAgent, 300)
	event.Details = sanitizeDetails(event.Details)
	return event
}

func sanitizeDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	sanitized := make(map[string]any, len(details))
	for key, value := range details {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "apikey") || strings.Contains(lower, "api_key") || strings.Contains(lower, "secret") {
			sanitized[key] = "[redacted]"
			continue
		}
		switch typed := value.(type) {
		case string:
			sanitized[key] = truncate(typed, 1200)
		default:
			sanitized[key] = typed
		}
	}
	return sanitized
}

func sanitizePath(path string) string {
	if path == "" {
		return ""
	}
	if index := strings.Index(path, "?"); index >= 0 {
		return path[:index]
	}
	return path
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func newID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "event-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buffer)
}
