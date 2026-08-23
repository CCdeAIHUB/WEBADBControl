package automation

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

type blockingExecutor struct{ started chan struct{} }

func (executor blockingExecutor) ExecuteAutomationAction(ctx context.Context, _ Task, _ Action) error {
	select {
	case executor.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestControlPersistsPauseResumeAndStopStates(t *testing.T) {
	// 场景：暂停/继续/停止必须更新持久化运行状态，不能只切换内存布尔值后仍向 UI 显示 running。
	repository, err := OpenRepository(filepath.Join(t.TempDir(), "automation.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	task := Task{ID: "task-control", Name: "控制测试", SchemaVersion: 1, ConcurrencyPolicy: "skip", Triggers: []Trigger{{Type: "manual"}}, Actions: []Action{{Type: "log"}}}
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 1)
	service := NewService(repository, blockingExecutor{started: started})
	run, err := service.Run(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not start")
	}

	for _, test := range []struct {
		operation string
		status    RunStatus
	}{{"pause", RunPaused}, {"resume", RunRunning}, {"stop", RunStopped}} {
		if err := service.Control(run.ID, test.operation); err != nil {
			t.Fatalf("%s: %v", test.operation, err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			runs, err := repository.ListRuns(context.Background(), 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(runs) > 0 && runs[0].Status == test.status {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s status = %#v, want %s", test.operation, runs, test.status)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
