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
	retainedRecord      bool
	versionCode         int64
	versionName         string
	installCalls        int
	uninstallCalls      int
	commandOrder        []string
	installOutput       CommandOutput
	uninstallOutput     CommandOutput
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
	case strings.Contains(joined, "shell pm list packages "+companionPackageName):
		if c.installed {
			output.Stdout = "package:" + companionPackageName + "\n"
		}
	case strings.Contains(joined, "shell dumpsys package "+companionPackageName):
		if c.installed || c.retainedRecord {
			output.Stdout = "Packages:\n  Package [" + companionPackageName + "]:\n    versionCode=" + formatVersionCode(c.versionCode) + " minSdk=29 targetSdk=36\n    versionName=" + c.versionName + "\n"
		} else {
			output.Stdout = "Unable to find package: " + companionPackageName + "\n"
		}
	case strings.Contains(joined, " install -r "):
		c.installCalls++
		c.commandOrder = append(c.commandOrder, "install")
		output = c.installOutput
		if output.ExitCode == 0 && !c.keepOldAfterInstall {
			c.installed = true
			c.versionCode = 13
			c.versionName = "0.13.0"
		}
	case strings.Contains(joined, " uninstall "+companionPackageName):
		c.uninstallCalls++
		c.commandOrder = append(c.commandOrder, "uninstall")
		output = c.uninstallOutput
		if output.ExitCode == 0 {
			c.installed = false
		}
	case strings.Contains(joined, " install "):
		c.installCalls++
		c.commandOrder = append(c.commandOrder, "install")
		output = c.installOutput
		if output.ExitCode == 0 {
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
	// 场景：用户确认覆盖安装后，旧版伴侣使用 install -r 升级并重新读取版本。
	caller := &companionUpgradeCaller{installed: true, versionCode: 11, versionName: "0.11.0"}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	result, err := service.ForceInstallConfiguredCompanion(context.Background(), "phone", "manual-confirmed")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.PreviousVersionCode != 11 || result.InstalledVersionCode != 13 || caller.installCalls != 1 {
		t.Fatalf("unexpected result=%#v installCalls=%d", result, caller.installCalls)
	}
}

func TestEnsureCompanionSkipsCurrentPackage(t *testing.T) {
	// 场景：设备版本满足能力基线时，能力入口不执行安装。
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
	// 场景：用户确认安装后，缺失的伴侣使用服务端内置 APK 安装并复检。
	caller := &companionUpgradeCaller{}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	result, err := service.ForceInstallConfiguredCompanion(context.Background(), "phone", "manual-confirmed")
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
	_, err := service.ForceInstallConfiguredCompanion(context.Background(), "phone", "manual-confirmed")
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.ErrorCode != "COMPANION_SIGNATURE_MISMATCH" {
		t.Fatalf("error=%v, want COMPANION_SIGNATURE_MISMATCH", err)
	}
	if caller.installCalls != 1 || caller.uninstallCalls != 0 {
		t.Fatalf("installCalls=%d uninstallCalls=%d, want 1/0 before user confirmation", caller.installCalls, caller.uninstallCalls)
	}
}

func TestReinstallCompanionUninstallsOnlyAfterExplicitConfirmation(t *testing.T) {
	// 场景：前端已取得用户的破坏性操作确认后，才卸载签名冲突的旧包，再安装内置 APK 并复检。
	caller := &companionUpgradeCaller{installed: true, versionCode: 11, versionName: "0.11.0"}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	result, err := service.ReinstallConfiguredCompanion(context.Background(), "phone", "signature-mismatch-confirmed")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.State != CompanionUpgradeReinstalled || result.InstalledVersionCode != 13 {
		t.Fatalf("unexpected result=%#v", result)
	}
	if strings.Join(caller.commandOrder, ",") != "uninstall,install" {
		t.Fatalf("command order=%v, want uninstall then install", caller.commandOrder)
	}
}

func TestReinstallCompanionValidatesAPKBeforeUninstall(t *testing.T) {
	// 场景：服务端 APK 丢失时绝不能先卸载设备中仍可用的伴侣应用。
	caller := &companionUpgradeCaller{installed: true, versionCode: 11, versionName: "0.11.0"}
	service := NewService(caller, WithCompanionRequirement(CompanionRequirement{APKPath: filepath.Join(t.TempDir(), "missing.apk"), VersionCode: 13, VersionName: "0.13.0"}))
	_, err := service.ReinstallConfiguredCompanion(context.Background(), "phone", "signature-mismatch-confirmed")
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.ErrorCode != "COMPANION_APK_MISSING" {
		t.Fatalf("error=%v, want COMPANION_APK_MISSING", err)
	}
	if caller.uninstallCalls != 0 || caller.installCalls != 0 {
		t.Fatalf("uninstallCalls=%d installCalls=%d, want 0/0", caller.uninstallCalls, caller.installCalls)
	}
}

func TestReinstallCompanionStopsWhenUninstallFails(t *testing.T) {
	// 场景：卸载阶段失败时保留旧应用并停止流程，不能继续安装造成误导性的二次错误。
	caller := &companionUpgradeCaller{
		installed: true, versionCode: 11, versionName: "0.11.0",
		uninstallOutput: CommandOutput{ExitCode: 1, Stderr: "Failure [DELETE_FAILED_DEVICE_POLICY_MANAGER]"},
	}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	_, err := service.ReinstallConfiguredCompanion(context.Background(), "phone", "signature-mismatch-confirmed")
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.ErrorCode != "COMPANION_UNINSTALL_FAILED" {
		t.Fatalf("error=%v, want COMPANION_UNINSTALL_FAILED", err)
	}
	if caller.uninstallCalls != 1 || caller.installCalls != 0 || !caller.installed {
		t.Fatalf("uninstallCalls=%d installCalls=%d installed=%v", caller.uninstallCalls, caller.installCalls, caller.installed)
	}
}

func TestEnsureCompanionRejectsUnverifiedUpgrade(t *testing.T) {
	// 场景：ADB 报告安装成功但设备版本仍未更新时，流程必须失败，不能把旧伴侣标记为可用。
	caller := &companionUpgradeCaller{installed: true, versionCode: 11, versionName: "0.11.0", keepOldAfterInstall: true}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	_, err := service.ForceInstallConfiguredCompanion(context.Background(), "phone", "manual-confirmed")
	if err == nil || !strings.Contains(err.Error(), "COMPANION_UPGRADE_VERIFY_FAILED") {
		t.Fatalf("error=%v, want COMPANION_UPGRADE_VERIFY_FAILED", err)
	}
}

func TestEnsureCompanionRequiresConfirmationWithoutInstalling(t *testing.T) {
	// 场景：功能发现伴侣过旧时只返回可识别的确认错误，不得擅自覆盖安装。
	caller := &companionUpgradeCaller{installed: true, versionCode: 11, versionName: "0.11.0"}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	_, err := service.EnsureConfiguredCompanion(context.Background(), "phone", "capabilities")
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.ErrorCode != "COMPANION_UPGRADE_REQUIRED" {
		t.Fatalf("error=%v, want COMPANION_UPGRADE_REQUIRED", err)
	}
	if caller.installCalls != 0 {
		t.Fatalf("installCalls=%d, want 0 before user confirmation", caller.installCalls)
	}
}

func TestProbeCompanionTreatsRetainedUninstalledRecordAsMissing(t *testing.T) {
	// 场景：用户卸载后 dumpsys 仍保留旧签名包记录，正常安装列表为空时必须判定为未安装。
	caller := &companionUpgradeCaller{retainedRecord: true, versionCode: 11, versionName: "0.11.0"}
	service := NewService(caller, WithCompanionRequirement(testCompanionRequirement(t)))
	result, err := service.CheckConfiguredCompanion(context.Background(), "phone", "status")
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CompanionUpgradeMissing || result.InstalledVersionCode != 0 {
		t.Fatalf("unexpected result=%#v", result)
	}
}
