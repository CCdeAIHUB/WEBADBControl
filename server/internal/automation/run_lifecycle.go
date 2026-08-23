package automation

import (
	"context"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

func (s *Service) transitionRun(control *runControl, next RunStatus) error {
	control.mu.Lock()
	if control.run.Status == next {
		control.mu.Unlock()
		return nil
	}
	if err := control.run.Transition(next); err != nil {
		control.mu.Unlock()
		return apperror.Wrap("AUTOMATION_STATE_INVALID", "自动化运行状态转换失败", "automation.runtime", false, err)
	}
	snapshot := *control.run
	control.mu.Unlock()
	if err := s.repository.SaveRun(context.Background(), snapshot); err != nil {
		return apperror.Wrap("AUTOMATION_RUN_SAVE_FAILED", "自动化运行状态保存失败", "automation.repository", true, err)
	}
	return nil
}

func (s *Service) updateRunProgress(control *runControl, completed int) error {
	control.mu.Lock()
	control.run.CompletedSteps = completed
	control.run.Progress = float64(completed) / float64(control.run.TotalSteps)
	snapshot := *control.run
	control.mu.Unlock()
	if err := s.repository.SaveRun(context.Background(), snapshot); err != nil {
		return apperror.Wrap("AUTOMATION_RUN_SAVE_FAILED", "自动化运行进度保存失败", "automation.repository", true, err)
	}
	return nil
}

func (s *Service) logRunFailure(event string, control *runControl, err error) {
	control.mu.Lock()
	runID, taskID := control.run.ID, control.run.TaskID
	control.mu.Unlock()
	s.logger.Error(event, "runId", runID, "taskId", taskID, "error", err)
}
