package automation

import "testing"

func TestValidateRequiresExplicitDevicePermissions(t *testing.T) {
	// 场景：ADB shell 与 Companion 动作必须声明对应权限，不能由任务隐式提权。
	task := Task{
		Name:     "设备检查",
		Triggers: []Trigger{{Type: "manual"}},
		Actions:  []Action{{Type: "adb.shell", Parameters: map[string]any{"command": "getprop ro.product.model"}}},
	}
	if err := Validate(task); err == nil {
		t.Fatal("expected missing ADB permission to fail")
	}
	task.Permissions.AllowADB = true
	task.Permissions.AllowShell = true
	if err := Validate(task); err != nil {
		t.Fatal(err)
	}
}

func TestRunStateMachineRejectsIllegalTransition(t *testing.T) {
	// 场景：任务成功后不能重新进入运行态，非法状态迁移必须显式失败。
	run := Run{Status: RunQueued}
	for _, next := range []RunStatus{RunRunning, RunSucceeded} {
		if err := run.Transition(next); err != nil {
			t.Fatal(err)
		}
	}
	if err := run.Transition(RunRunning); err == nil {
		t.Fatal("expected terminal state transition to fail")
	}
}

func TestRunStateMachineAllowsPauseResumeAndStop(t *testing.T) {
	// 场景：运行中任务必须允许暂停、继续和停止，并保留显式状态。
	run := Run{Status: RunQueued}
	for _, next := range []RunStatus{RunRunning, RunPaused, RunRunning, RunStopped} {
		if err := run.Transition(next); err != nil {
			t.Fatalf("transition to %s failed: %v", next, err)
		}
	}
}
