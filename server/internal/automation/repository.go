package automation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Repository struct{ database *sql.DB }

func OpenRepository(path string) (*Repository, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	repository := &Repository{database: database}
	if err := repository.migrate(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	return repository, nil
}

func (r *Repository) migrate(ctx context.Context) error {
	_, err := r.database.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS automation_tasks (
  id TEXT PRIMARY KEY,
  definition_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS automation_runs (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  run_json TEXT NOT NULL,
  started_at TEXT NOT NULL,
  FOREIGN KEY(task_id) REFERENCES automation_tasks(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_automation_runs_task_started ON automation_runs(task_id, started_at DESC);
`)
	return err
}

func (r *Repository) ListTasks(ctx context.Context) ([]Task, error) {
	rows, err := r.database.QueryContext(ctx, "SELECT definition_json FROM automation_tasks ORDER BY updated_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := make([]Task, 0)
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var task Task
		if err := json.Unmarshal([]byte(encoded), &task); err != nil {
			return nil, fmt.Errorf("decode task: %w", err)
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *Repository) GetTask(ctx context.Context, id string) (Task, error) {
	var encoded string
	if err := r.database.QueryRowContext(ctx, "SELECT definition_json FROM automation_tasks WHERE id = ?", id).Scan(&encoded); err != nil {
		return Task{}, err
	}
	var task Task
	if err := json.Unmarshal([]byte(encoded), &task); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (r *Repository) SaveTask(ctx context.Context, task Task) error {
	encoded, err := json.Marshal(task)
	if err != nil {
		return err
	}
	_, err = r.database.ExecContext(ctx, `INSERT INTO automation_tasks(id, definition_json, created_at, updated_at)
VALUES(?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET definition_json=excluded.definition_json, updated_at=excluded.updated_at`,
		task.ID, string(encoded), task.CreatedAt.Format(time.RFC3339Nano), task.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

func (r *Repository) DeleteTask(ctx context.Context, id string) error {
	_, err := r.database.ExecContext(ctx, "DELETE FROM automation_tasks WHERE id = ?", id)
	return err
}

func (r *Repository) SaveRun(ctx context.Context, run Run) error {
	encoded, err := json.Marshal(run)
	if err != nil {
		return err
	}
	started := run.StartedAt
	if started.IsZero() {
		started = time.Now().UTC()
	}
	_, err = r.database.ExecContext(ctx, `INSERT INTO automation_runs(id, task_id, run_json, started_at)
VALUES(?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET run_json=excluded.run_json`, run.ID, run.TaskID, string(encoded), started.Format(time.RFC3339Nano))
	return err
}

func (r *Repository) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.database.QueryContext(ctx, "SELECT run_json FROM automation_runs ORDER BY started_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]Run, 0)
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var run Run
		if err := json.Unmarshal([]byte(encoded), &run); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *Repository) RecoverInterrupted(ctx context.Context) error {
	runs, err := r.ListRuns(ctx, 200)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.Status == RunQueued || run.Status == RunRunning || run.Status == RunPaused {
			run.Status = RunFailed
			run.ErrorCode = "APP_RESTART_INTERRUPTED"
			run.FinishedAt = time.Now().UTC()
			if err := r.SaveRun(ctx, run); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) Close() error { return r.database.Close() }
