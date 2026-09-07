package observability

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const maxEvents = 20000

type Repository struct{ database *sql.DB }

func OpenRepository(path string) (*Repository, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	repository := &Repository{database: database}
	if err := repository.migrate(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	return repository, nil
}

func (r *Repository) migrate(ctx context.Context) error {
	_, err := r.database.ExecContext(ctx, `
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=3000;
CREATE TABLE IF NOT EXISTS observability_events (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  level TEXT NOT NULL,
  module TEXT NOT NULL,
  action TEXT NOT NULL,
  message TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  error_code TEXT NOT NULL DEFAULT '',
  device_id TEXT NOT NULL DEFAULT '',
  method TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT '',
  status INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  actor TEXT NOT NULL DEFAULT '',
  ip_address TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  details_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_observability_events_created ON observability_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_observability_events_trace ON observability_events(trace_id);
CREATE INDEX IF NOT EXISTS idx_observability_events_type_level ON observability_events(type, level, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_observability_events_device ON observability_events(device_id, created_at DESC);
`)
	return err
}

func (r *Repository) Save(ctx context.Context, event Event) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	details := "{}"
	if len(event.Details) > 0 {
		encoded, err := json.Marshal(event.Details)
		if err != nil {
			return fmt.Errorf("encode observability details: %w", err)
		}
		details = string(encoded)
	}
	_, err := r.database.ExecContext(ctx, `INSERT INTO observability_events(
id, type, level, module, action, message, trace_id, error_code, device_id, method, path, status, duration_ms, actor, ip_address, user_agent, details_json, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.Type, event.Level, event.Module, event.Action, event.Message, event.TraceID, event.ErrorCode, event.DeviceID,
		event.Method, event.Path, event.Status, event.DurationMS, event.Actor, event.IPAddress, event.UserAgent, details, event.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	// The router deployment has limited storage. Keep recent diagnostic value
	// while bounding the SQLite table so logs do not starve application data.
	_, _ = r.database.ExecContext(ctx, `DELETE FROM observability_events WHERE id IN (
SELECT id FROM observability_events ORDER BY created_at DESC LIMIT -1 OFFSET ?
)`, maxEvents)
	return nil
}

func (r *Repository) List(ctx context.Context, query Query) ([]Event, error) {
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	clauses := make([]string, 0)
	args := make([]any, 0)
	addEqual := func(column, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		clauses = append(clauses, column+" = ?")
		args = append(args, value)
	}
	addEqual("type", query.Type)
	addEqual("level", query.Level)
	addEqual("module", query.Module)
	addEqual("action", query.Action)
	addEqual("trace_id", query.TraceID)
	addEqual("error_code", query.ErrorCode)
	addEqual("device_id", query.DeviceID)
	if text := strings.TrimSpace(query.Text); text != "" {
		clauses = append(clauses, "(message LIKE ? OR path LIKE ? OR details_json LIKE ?)")
		like := "%" + text + "%"
		args = append(args, like, like, like)
	}
	statement := `SELECT id, type, level, module, action, message, trace_id, error_code, device_id, method, path, status, duration_ms, actor, ip_address, user_agent, details_json, created_at FROM observability_events`
	if len(clauses) > 0 {
		statement += " WHERE " + strings.Join(clauses, " AND ")
	}
	statement += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := r.database.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]Event, 0)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *Repository) Stats(ctx context.Context) (Stats, error) {
	stats := Stats{ByType: map[string]int64{}, ByLevel: map[string]int64{}}
	if err := r.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM observability_events").Scan(&stats.Total); err != nil {
		return stats, err
	}
	if err := r.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM observability_events WHERE level = 'error'").Scan(&stats.ErrorCount); err != nil {
		return stats, err
	}
	if err := r.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM observability_events WHERE level = 'warn'").Scan(&stats.WarnCount); err != nil {
		return stats, err
	}
	var lastError sql.NullString
	if err := r.database.QueryRowContext(ctx, "SELECT MAX(created_at) FROM observability_events WHERE level = 'error'").Scan(&lastError); err != nil {
		return stats, err
	}
	if lastError.Valid {
		stats.LastErrorAt = lastError.String
	}
	if err := scanCounts(ctx, r.database, "SELECT type, COUNT(*) FROM observability_events GROUP BY type", stats.ByType); err != nil {
		return stats, err
	}
	if err := scanCounts(ctx, r.database, "SELECT level, COUNT(*) FROM observability_events GROUP BY level", stats.ByLevel); err != nil {
		return stats, err
	}
	return stats, nil
}

func scanCounts(ctx context.Context, database *sql.DB, statement string, target map[string]int64) error {
	rows, err := database.QueryContext(ctx, statement)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int64
		if err := rows.Scan(&key, &count); err != nil {
			return err
		}
		target[key] = count
	}
	return rows.Err()
}

type eventScanner interface {
	Scan(dest ...any) error
}

func scanEvent(scanner eventScanner) (Event, error) {
	var event Event
	var details, created string
	if err := scanner.Scan(&event.ID, &event.Type, &event.Level, &event.Module, &event.Action, &event.Message, &event.TraceID, &event.ErrorCode, &event.DeviceID,
		&event.Method, &event.Path, &event.Status, &event.DurationMS, &event.Actor, &event.IPAddress, &event.UserAgent, &details, &created); err != nil {
		return Event{}, err
	}
	if details != "" && details != "{}" {
		_ = json.Unmarshal([]byte(details), &event.Details)
	}
	parsed, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Event{}, err
	}
	event.CreatedAt = parsed
	return event, nil
}

func (r *Repository) Close() error { return r.database.Close() }
