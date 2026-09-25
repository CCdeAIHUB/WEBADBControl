package device

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

type companionUpgradeCaller struct {
	installed           bool
	versionCode         int64
	versionName         string
	installCalls        int
	installOutput       CommandOutput
	keepOldAfterInstall bool
}

func (c *companionUpgradeCaller) Call(_ context.Context, method string, params any, target any) error {
	if method != "adb.exec" {
		return errors.New("unexpected method: " + method)
	}
	args := params.(map[string]any)["args"].([]string)
	output := CommandOutput{}
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "shell dumpsys package "+companionPackageName):
		if c.installed {
			output.Stdout = "Packages:\n  Package [" + companionPackageName + "]:\n    versionCode=" + formatVersionCode(c.versionCode) + " minSdk=29 targetSdk=36\n    versionName=" + c.versionName + "\n"
		} else {
			output.Stdout = "Unable to find package: " + companionPackageName + "\n"
		}
	case strings.Contains(joined, " install -r "):
		c.installCalls++
		output = c.installOutput
		if output.ExitCode == 0 && !c.keepOldAfterInstall {
			c.installed = true
			c.versionCode = 13
			c.versionName = "0.13.0"
		}
	default:
		return errors.New("unexpected adb args: " + joined)
	}
	*(target.(*CommandOutput)) = output
	return nil
}

func formatVersionCode(value int64) string {
	return strconv.FormatInt(value, 10)
}

func testCompanionRequirement(t *testing.T) CompanionRequirement {
	t.Helper()
	apkPath := filepath.Join(t.TempDir(), "companion.apk")
	if err := os.WriteFile(apkPath, []byte("test apk"), 0o600); err != nil {
		t.Fatal(err)
	}
	return CompanionRequirement{APKPath: apkPath, VersionCode: 13, VersionName: "0.13.0"}
}

func TestEnsureCompanionUpgradesOutdatedPackageAndVerifiesVersion(t *testing.T) {
	// 场景：设备安装旧版伴侣时，能力入口必须使用 install -r 覆盖升级，并在成功后重新读取版本。
	caller := &companionUpgradeCaller{installed: true, versionCode: 11, versionName: "0.11.0"}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	result, err := service.EnsureConfiguredCompanion(context.Background(), "phone", "capabilities")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.PreviousVersionCode != 11 || result.InstalledVersionCode != 13 || caller.installCalls != 1 {
		t.Fatalf("unexpected result=%#v installCalls=%d", result, caller.installCalls)
	}
}

func TestEnsureCompanionSkipsCurrentPackage(t *testing.T) {
	// 场景：设备版本已经满足最低能力版本时，任何入口都不能重复覆盖安装。
	caller := &companionUpgradeCaller{installed: true, versionCode: 13, versionName: "0.13.0"}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	result, err := service.EnsureConfiguredCompanion(context.Background(), "phone", "screen")
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || caller.installCalls != 0 || result.State != CompanionUpgradeReady {
		t.Fatalf("unexpected result=%#v installCalls=%d", result, caller.installCalls)
	}
}

func TestEnsureCompanionInstallsMissingPackage(t *testing.T) {
	// 场景：伴侣缺失时允许直接安装服务端内置 APK，且安装后必须达到要求版本。
	caller := &companionUpgradeCaller{}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	result, err := service.EnsureConfiguredCompanion(context.Background(), "phone", "screen")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.State != CompanionUpgradeInstalled || caller.installCalls != 1 {
		t.Fatalf("unexpected result=%#v installCalls=%d", result, caller.installCalls)
	}
}

func TestEnsureCompanionNeverUninstallsOnSignatureMismatch(t *testing.T) {
	// 场景：签名不一致时 install -r 必须失败并给出稳定错误码，绝不尝试卸载以绕过签名保护。
	caller := &companionUpgradeCaller{
		installed: true, versionCode: 11, versionName: "0.11.0",
		installOutput: CommandOutput{ExitCode: 1, Stderr: "INSTALL_FAILED_UPDATE_INCOMPATIBLE: signatures do not match"},
	}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	_, err := service.EnsureConfiguredCompanion(context.Background(), "phone", "screen")
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.ErrorCode != "COMPANION_SIGNATURE_MISMATCH" {
		t.Fatalf("error=%v, want COMPANION_SIGNATURE_MISMATCH", err)
	}
	if caller.installCalls != 1 {
		t.Fatalf("installCalls=%d, want 1", caller.installCalls)
	}
}

func TestEnsureCompanionRejectsUnverifiedUpgrade(t *testing.T) {
	// 场景：ADB 报告安装成功但设备版本仍未更新时，流程必须失败，不能把旧伴侣标记为可用。
	caller := &companionUpgradeCaller{installed: true, versionCode: 11, versionName: "0.11.0", keepOldAfterInstall: true}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	_, err := service.EnsureConfiguredCompanion(context.Background(), "phone", "screen")
	if err == nil || !strings.Contains(err.Error(), "COMPANION_UPGRADE_VERIFY_FAILED") {
		t.Fatalf("error=%v, want COMPANION_UPGRADE_VERIFY_FAILED", err)
	}
}
