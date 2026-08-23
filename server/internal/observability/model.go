package observability

import "time"

type Event struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Level      string         `json:"level"`
	Module     string         `json:"module"`
	Action     string         `json:"action"`
	Message    string         `json:"message"`
	TraceID    string         `json:"traceId"`
	ErrorCode  string         `json:"errorCode,omitempty"`
	DeviceID   string         `json:"deviceId,omitempty"`
	Method     string         `json:"method,omitempty"`
	Path       string         `json:"path,omitempty"`
	Status     int            `json:"status,omitempty"`
	DurationMS int64          `json:"durationMs,omitempty"`
	Actor      string         `json:"actor,omitempty"`
	IPAddress  string         `json:"ipAddress,omitempty"`
	UserAgent  string         `json:"userAgent,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
}

type Query struct {
	Type      string
	Level     string
	Module    string
	Action    string
	TraceID   string
	ErrorCode string
	DeviceID  string
	Text      string
	Limit     int
}

type Stats struct {
	Total       int64            `json:"total"`
	ErrorCount  int64            `json:"errorCount"`
	WarnCount   int64            `json:"warnCount"`
	ByType      map[string]int64 `json:"byType"`
	ByLevel     map[string]int64 `json:"byLevel"`
	LastErrorAt string           `json:"lastErrorAt,omitempty"`
}
