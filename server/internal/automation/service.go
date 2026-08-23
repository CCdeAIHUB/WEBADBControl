package automation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

type ActionExecutor interface {
	ExecuteAutomationAction(context.Context, Task, Action) error
}

type Service struct {
	repository *Repository
	executor   ActionExecutor
	logger     *slog.Logger
	mu         sync.Mutex
	controls   map[string]*runControl
}

type runControl struct {
	mu     sync.Mutex
	paused bool
	resume chan struct{}
	cancel context.CancelFunc
	run    *Run
}

func NewService(repository *Repository, executor ActionExecutor) *Service {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Service{repository: repository, executor: executor, logger: logger, controls: map[string]*runControl{}}
}

func (s *Service) SetLogger(logger *slog.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

func (s *Service) List(ctx context.Context) ([]Task, error) { return s.repository.ListTasks(ctx) }
func (s *Service) Runs(ctx context.Context) ([]Run, error)  { return s.repository.ListRuns(ctx, 50) }

func (s *Service) Save(ctx context.Context, task Task) (Task, error) {
	if err := Validate(task); err != nil {
		return Task{}, apperror.Wrap("AUTOMATION_TASK_INVALID", "自动化任务定义无效", "automation.validation", true, err)
	}
	now := time.Now().UTC()
	if task.ID == "" {
		identifier, err := identifier("task")
		if err != nil {
			return Task{}, err
		}
		task.ID = identifier
		task.CreatedAt = now
	}
	if task.SchemaVersion == 0 {
		task.SchemaVersion = 1
	}
	if task.ConcurrencyPolicy == "" {
		task.ConcurrencyPolicy = "skip"
	}
	task.UpdatedAt = now
	if err := s.repository.SaveTask(ctx, task); err != nil {
		return Task{}, apperror.Wrap("AUTOMATION_SAVE_FAILED", "保存自动化任务失败", "automation.repository", true, err)
	}
	return task, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repository.DeleteTask(ctx, id)
}

func (s *Service) Run(ctx context.Context, taskID string) (Run, error) {
	task, err := s.repository.GetTask(ctx, taskID)
	if err != nil {
		return Run{}, apperror.Wrap("AUTOMATION_TASK_NOT_FOUND", "自动化任务不存在", "automation.repository", true, err)
	}
	s.mu.Lock()
	if task.ConcurrencyPolicy == "skip" {
		for runID := range s.controls {
			// One active run per task is enforced by the run map key prefix below.
			if len(runID) > 0 && activeTask(runID) == taskID {
				s.mu.Unlock()
				return Run{}, apperror.New("AUTOMATION_CONCURRENCY_SKIPPED", "任务已有运行实例", "automation.runtime", true)
			}
		}
	}
	runIdentifier, err := identifier("run")
	if err != nil {
		s.mu.Unlock()
		return Run{}, err
	}
	run := Run{ID: taskID + ":" + runIdentifier, TaskID: taskID, Status: RunQueued, TotalSteps: len(task.Actions)}
	runCtx, cancel := context.WithCancel(context.Background())
	control := &runControl{resume: make(chan struct{}), cancel: cancel, run: &run}
	s.controls[run.ID] = control
	s.mu.Unlock()
	if err := s.repository.SaveRun(ctx, run); err != nil {
		cancel()
		s.mu.Lock()
		delete(s.controls, run.ID)
		s.mu.Unlock()
		return Run{}, apperror.Wrap("AUTOMATION_RUN_SAVE_FAILED", "创建自动化运行记录失败", "automation.repository", true, err)
	}
	go s.execute(runCtx, task, &run, control)
	return run, nil
}

func (s *Service) execute(ctx context.Context, task Task, _ *Run, control *runControl) {
	if err := s.transitionRun(control, RunRunning); err != nil {
		s.logRunFailure("automation_run_start_failed", control, err)
		return
	}
	defer func() {
		s.mu.Lock()
		delete(s.controls, control.run.ID)
		s.mu.Unlock()
	}()
	for index, action := range task.Actions {
		if err := control.wait(ctx); err != nil {
			if transitionErr := s.transitionRun(control, RunStopped); transitionErr != nil {
				s.logRunFailure("automation_run_stop_failed", control, transitionErr)
			}
			return
		}
		if err := s.executor.ExecuteAutomationAction(ctx, task, action); err != nil {
			if ctx.Err() != nil {
				if transitionErr := s.transitionRun(control, RunStopped); transitionErr != nil {
					s.logRunFailure("automation_run_stop_failed", control, transitionErr)
				}
				return
			}
			control.mu.Lock()
			control.run.ErrorCode = "AUTOMATION_ACTION_FAILED"
			control.mu.Unlock()
			if transitionErr := s.transitionRun(control, RunFailed); transitionErr != nil {
				s.logRunFailure("automation_run_failure_save_failed", control, transitionErr)
			}
			return
		}
		if err := s.updateRunProgress(control, index+1); err != nil {
			s.logRunFailure("automation_run_progress_save_failed", control, err)
			return
		}
	}
	if err := s.transitionRun(control, RunSucceeded); err != nil {
		s.logRunFailure("automation_run_finish_failed", control, err)
	}
}

func (c *runControl) wait(ctx context.Context) error {
	for {
		c.mu.Lock()
		paused, resume := c.paused, c.resume
		c.mu.Unlock()
		if !paused {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-resume:
		}
	}
}

func (s *Service) Control(runID, operation string) error {
	s.mu.Lock()
	control, ok := s.controls[runID]
	s.mu.Unlock()
	if !ok {
		return apperror.New("AUTOMATION_RUN_NOT_ACTIVE", "运行实例当前不可控制", "automation.runtime", true)
	}
	switch operation {
	case "pause":
		control.mu.Lock()
		if control.paused {
			control.mu.Unlock()
			return nil
		}
		control.paused = true
		control.mu.Unlock()
		if err := s.transitionRun(control, RunPaused); err != nil {
			control.mu.Lock()
			control.paused = false
			control.mu.Unlock()
			return err
		}
	case "resume":
		control.mu.Lock()
		if !control.paused {
			control.mu.Unlock()
			return nil
		}
		control.paused = false
		close(control.resume)
		control.resume = make(chan struct{})
		control.mu.Unlock()
		if err := s.transitionRun(control, RunRunning); err != nil {
			return err
		}
	case "stop":
		if err := s.transitionRun(control, RunStopped); err != nil {
			return err
		}
		control.cancel()
	default:
		return apperror.New("AUTOMATION_CONTROL_INVALID", "不支持的运行控制操作", "automation.runtime", true)
	}
	return nil
}

func identifier(prefix string) (string, error) {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", apperror.Wrap("AUTOMATION_ID_FAILED", "无法生成自动化标识", "automation.runtime", false, err)
	}
	return prefix + "-" + hex.EncodeToString(buffer), nil
}

func activeTask(runID string) string {
	for index := 0; index < len(runID); index++ {
		if runID[index] == ':' {
			return runID[:index]
		}
	}
	return ""
}
