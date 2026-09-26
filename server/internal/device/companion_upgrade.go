package device

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

const (
	CompanionUpgradeReady       = "ready"
	CompanionUpgradeMissing     = "missing"
	CompanionUpgradeOutdated    = "outdated"
	CompanionUpgradeInstalled   = "installed"
	CompanionUpgradeUpdated     = "updated"
	CompanionUpgradeReinstalled = "reinstalled"
)

type CompanionRequirement struct {
	APKPath     string
	VersionCode int64
	VersionName string
}

type CompanionPackageInfo struct {
	Installed   bool   `json:"installed"`
	VersionCode int64  `json:"versionCode,omitempty"`
	VersionName string `json:"versionName,omitempty"`
}

type CompanionUpgradeResult struct {
	State                string `json:"state"`
	Updated              bool   `json:"updated"`
	PreviousVersionCode  int64  `json:"previousVersionCode,omitempty"`
	PreviousVersionName  string `json:"previousVersionName,omitempty"`
	InstalledVersionCode int64  `json:"installedVersionCode"`
	InstalledVersionName string `json:"installedVersionName"`
	RequiredVersionCode  int64  `json:"requiredVersionCode"`
	RequiredVersionName  string `json:"requiredVersionName"`
}

var (
	companionVersionCodePattern = regexp.MustCompile(`(?m)\bversionCode=(\d+)\b`)
	companionVersionNamePattern = regexp.MustCompile(`(?m)^\s*versionName=([^\s]+)\s*$`)
)

func (r CompanionRequirement) configured() bool {
	return strings.TrimSpace(r.APKPath) != "" && r.VersionCode > 0
}

func (s *Service) ProbeCompanionPackage(ctx context.Context, deviceID string) (CompanionPackageInfo, error) {
	// `dumpsys package` retains records after a user uninstalls an app and marks
	// them installed=false. The normal package list is the source of truth for
	// whether the package is currently installed for the active Android user.
	listed, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "pm", "list", "packages", companionPackageName))
	if err != nil {
		return CompanionPackageInfo{}, err
	}
	if !packageListContains(listed.Stdout, companionPackageName) {
		return CompanionPackageInfo{}, nil
	}
	output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "dumpsys", "package", companionPackageName))
	if err != nil {
		return CompanionPackageInfo{}, err
	}
	return parseCompanionPackageInfo(output.Stdout), nil
}

func packageListContains(output, packageName string) bool {
	want := "package:" + packageName
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func parseCompanionPackageInfo(output string) CompanionPackageInfo {
	normalized := strings.ReplaceAll(output, "\r", "")
	if strings.Contains(normalized, "Unable to find package:") || !strings.Contains(normalized, "Package ["+companionPackageName+"]") {
		return CompanionPackageInfo{}
	}
	info := CompanionPackageInfo{Installed: true}
	if match := companionVersionCodePattern.FindStringSubmatch(normalized); len(match) == 2 {
		info.VersionCode, _ = strconv.ParseInt(match[1], 10, 64)
	}
	if match := companionVersionNamePattern.FindStringSubmatch(normalized); len(match) == 2 {
		info.VersionName = strings.TrimSpace(match[1])
	}
	return info
}

func (s *Service) EnsureConfiguredCompanion(ctx context.Context, deviceID, trigger string) (CompanionUpgradeResult, error) {
	result, err := s.CheckConfiguredCompanion(ctx, deviceID, trigger)
	if err != nil {
		return CompanionUpgradeResult{}, err
	}
	switch result.State {
	case CompanionUpgradeMissing:
		return result, apperror.New("COMPANION_INSTALL_REQUIRED", "当前功能需要安装 ADBControl Companion", "companion.requirement", true).
			WithSuggestion("确认后可使用服务端内置 APK 进行安装")
	case CompanionUpgradeOutdated:
		return result, apperror.New("COMPANION_UPGRADE_REQUIRED", "当前伴侣版本不支持此功能", "companion.requirement", true).
			WithSuggestion(fmt.Sprintf("需要 versionCode %d，设备当前为 %d；确认后可安全覆盖安装", result.RequiredVersionCode, result.InstalledVersionCode))
	default:
		return result, nil
	}
}

func (s *Service) ForceInstallConfiguredCompanion(ctx context.Context, deviceID, trigger string) (CompanionUpgradeResult, error) {
	return s.installCompanion(ctx, deviceID, trigger)
}

// ReinstallConfiguredCompanion performs the destructive recovery path only
// after the web client has obtained explicit user confirmation. It validates
// the bundled APK before touching the package currently installed on Android.
func (s *Service) ReinstallConfiguredCompanion(ctx context.Context, deviceID, trigger string) (CompanionUpgradeResult, error) {
	requirement := s.companionRequirement
	if !requirement.configured() {
		return CompanionUpgradeResult{}, apperror.New("COMPANION_REQUIREMENT_NOT_CONFIGURED", "服务器未配置伴侣版本能力基线", "companion.upgrade", false)
	}

	lockValue, _ := s.companionUpgradeLocks.LoadOrStore(deviceID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	started := time.Now()
	if _, err := os.Stat(requirement.APKPath); err != nil {
		upgradeErr := apperror.Wrap("COMPANION_APK_MISSING", "服务器未打包 Android 伴侣应用", "companion.upgrade", false, err)
		s.logCompanionUpgrade("companion_reinstall_failed", deviceID, trigger, CompanionUpgradeResult{}, time.Since(started), upgradeErr)
		return CompanionUpgradeResult{}, upgradeErr
	}
	current, err := s.ProbeCompanionPackage(ctx, deviceID)
	if err != nil {
		return CompanionUpgradeResult{}, err
	}
	result := companionUpgradeResult(requirement, current)
	if current.Installed {
		output, uninstallErr := s.Exec(ctx, DeviceArgs(deviceID, "uninstall", companionPackageName))
		if uninstallErr != nil {
			reinstallErr := companionUninstallError(output, uninstallErr)
			s.logCompanionUpgrade("companion_uninstall_failed", deviceID, trigger, result, time.Since(started), reinstallErr)
			return CompanionUpgradeResult{}, reinstallErr
		}
	}
	output, installErr := s.Exec(ctx, DeviceArgs(deviceID, "install", requirement.APKPath))
	if installErr != nil {
		reinstallErr := companionReinstallError(output, installErr)
		s.logCompanionUpgrade("companion_reinstall_failed", deviceID, trigger, result, time.Since(started), reinstallErr)
		return CompanionUpgradeResult{}, reinstallErr
	}
	installed, err := s.ProbeCompanionPackage(ctx, deviceID)
	if err != nil || !installed.Installed || installed.VersionCode < requirement.VersionCode {
		cause := err
		if cause == nil {
			cause = fmt.Errorf("installed versionCode %d, required %d", installed.VersionCode, requirement.VersionCode)
		}
		reinstallErr := apperror.Wrap("COMPANION_REINSTALL_VERIFY_FAILED", "重新安装后伴侣版本仍不满足能力要求", "companion.upgrade", true, cause).
			WithSuggestion("请保持无线调试在线后重试安装；旧伴侣应用已经卸载")
		s.logCompanionUpgrade("companion_reinstall_verify_failed", deviceID, trigger, result, time.Since(started), reinstallErr)
		return CompanionUpgradeResult{}, reinstallErr
	}
	result.State = CompanionUpgradeReinstalled
	result.Updated = true
	result.InstalledVersionCode = installed.VersionCode
	result.InstalledVersionName = installed.VersionName
	s.logCompanionUpgrade("companion_reinstall_completed", deviceID, trigger, result, time.Since(started), nil)
	return result, nil
}

func (s *Service) CheckConfiguredCompanion(ctx context.Context, deviceID, trigger string) (CompanionUpgradeResult, error) {
	requirement := s.companionRequirement
	if !requirement.configured() {
		return CompanionUpgradeResult{State: CompanionUpgradeReady}, nil
	}
	current, err := s.ProbeCompanionPackage(ctx, deviceID)
	if err != nil {
		return CompanionUpgradeResult{}, err
	}
	result := companionUpgradeResult(requirement, current)
	if !current.Installed {
		result.State = CompanionUpgradeMissing
		s.logCompanionUpgrade("companion_requirement_missing", deviceID, trigger, result, 0, nil)
		return result, nil
	}
	if current.VersionCode < requirement.VersionCode {
		result.State = CompanionUpgradeOutdated
		s.logCompanionUpgrade("companion_requirement_outdated", deviceID, trigger, result, 0, nil)
		return result, nil
	}
	s.logCompanionUpgrade("companion_requirement_satisfied", deviceID, trigger, result, 0, nil)
	return result, nil
}

func companionUpgradeResult(requirement CompanionRequirement, current CompanionPackageInfo) CompanionUpgradeResult {
	return CompanionUpgradeResult{
		State:                CompanionUpgradeReady,
		PreviousVersionCode:  current.VersionCode,
		PreviousVersionName:  current.VersionName,
		InstalledVersionCode: current.VersionCode,
		InstalledVersionName: current.VersionName,
		RequiredVersionCode:  requirement.VersionCode,
		RequiredVersionName:  requirement.VersionName,
	}
}

func (s *Service) installCompanion(ctx context.Context, deviceID, trigger string) (CompanionUpgradeResult, error) {
	requirement := s.companionRequirement
	if !requirement.configured() {
		return CompanionUpgradeResult{}, apperror.New("COMPANION_REQUIREMENT_NOT_CONFIGURED", "服务器未配置伴侣版本能力基线", "companion.upgrade", false)
	}

	// A user may click confirm more than once while requests are in flight. Serialize
	// per device and probe again under the lock before the explicit install.
	lockValue, _ := s.companionUpgradeLocks.LoadOrStore(deviceID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	started := time.Now()
	current, err := s.ProbeCompanionPackage(ctx, deviceID)
	if err != nil {
		return CompanionUpgradeResult{}, err
	}
	result := companionUpgradeResult(requirement, current)
	if _, err := os.Stat(requirement.APKPath); err != nil {
		upgradeErr := apperror.Wrap("COMPANION_APK_MISSING", "服务器未打包 Android 伴侣应用", "companion.upgrade", false, err)
		s.logCompanionUpgrade("companion_upgrade_failed", deviceID, trigger, result, time.Since(started), upgradeErr)
		return CompanionUpgradeResult{}, upgradeErr
	}

	output, installErr := s.Exec(ctx, DeviceArgs(deviceID, "install", "-r", requirement.APKPath))
	if installErr != nil {
		upgradeErr := companionInstallError(output, installErr)
		s.logCompanionUpgrade("companion_upgrade_failed", deviceID, trigger, result, time.Since(started), upgradeErr)
		return CompanionUpgradeResult{}, upgradeErr
	}
	installed, err := s.ProbeCompanionPackage(ctx, deviceID)
	if err != nil {
		upgradeErr := apperror.Wrap("COMPANION_UPGRADE_VERIFY_FAILED", "伴侣覆盖安装后无法读取设备版本", "companion.upgrade", true, err).
			WithSuggestion("请保持无线调试在线后重试；系统不会卸载现有伴侣")
		s.logCompanionUpgrade("companion_upgrade_verify_failed", deviceID, trigger, result, time.Since(started), upgradeErr)
		return CompanionUpgradeResult{}, upgradeErr
	}
	if !installed.Installed || installed.VersionCode < requirement.VersionCode {
		upgradeErr := apperror.New("COMPANION_UPGRADE_VERIFY_FAILED", "伴侣覆盖安装后版本仍不满足能力要求", "companion.upgrade", true).
			WithSuggestion(fmt.Sprintf("需要 versionCode %d，设备当前为 %d", requirement.VersionCode, installed.VersionCode))
		s.logCompanionUpgrade("companion_upgrade_verify_failed", deviceID, trigger, result, time.Since(started), upgradeErr)
		return CompanionUpgradeResult{}, upgradeErr
	}
	result.Updated = true
	result.InstalledVersionCode = installed.VersionCode
	result.InstalledVersionName = installed.VersionName
	if current.Installed {
		result.State = CompanionUpgradeUpdated
	} else {
		result.State = CompanionUpgradeInstalled
	}
	s.logCompanionUpgrade("companion_upgrade_completed", deviceID, trigger, result, time.Since(started), nil)
	return result, nil
}

func companionInstallError(output CommandOutput, cause error) error {
	detail := strings.ToLower(strings.TrimSpace(output.Stderr + "\n" + output.Stdout))
	switch {
	case strings.Contains(detail, "install_failed_update_incompatible") || strings.Contains(detail, "signatures do not match"):
		return apperror.Wrap("COMPANION_SIGNATURE_MISMATCH", "Android 拒绝覆盖：设备中仍存在不同签名的伴侣应用", "companion.upgrade", true, cause).
			WithSuggestion("需要先卸载旧伴侣。系统会在你明确确认后执行卸载和重新安装")
	case strings.Contains(detail, "install_failed_version_downgrade"):
		return apperror.Wrap("COMPANION_REINSTALL_REQUIRED", "Android 不允许直接覆盖当前伴侣版本", "companion.upgrade", true, cause).
			WithSuggestion("需要先卸载旧伴侣。系统会在你明确确认后执行卸载和重新安装")
	case strings.Contains(detail, "device offline") || strings.Contains(detail, "device not found"):
		return apperror.Wrap("COMPANION_UPGRADE_DEVICE_OFFLINE", "伴侣升级时设备已离线", "companion.upgrade", true, cause).
			WithSuggestion("请保持无线调试页面开启并重新连接后重试")
	default:
		return apperror.Wrap("COMPANION_UPGRADE_FAILED", "伴侣覆盖安装失败", "companion.upgrade", true, cause).
			WithSuggestion(strings.TrimSpace(output.Stderr + " " + output.Stdout))
	}
}

func companionUninstallError(output CommandOutput, cause error) error {
	detail := strings.ToLower(strings.TrimSpace(output.Stderr + "\n" + output.Stdout))
	if strings.Contains(detail, "device offline") || strings.Contains(detail, "device not found") {
		return apperror.Wrap("COMPANION_REINSTALL_DEVICE_OFFLINE", "卸载旧伴侣时设备已离线", "companion.upgrade", true, cause).
			WithSuggestion("旧伴侣未被卸载；请恢复无线调试连接后重试")
	}
	return apperror.Wrap("COMPANION_UNINSTALL_FAILED", "无法卸载旧版伴侣应用", "companion.upgrade", true, cause).
		WithSuggestion(strings.TrimSpace(output.Stderr + " " + output.Stdout))
}

func companionReinstallError(output CommandOutput, cause error) error {
	detail := strings.ToLower(strings.TrimSpace(output.Stderr + "\n" + output.Stdout))
	if strings.Contains(detail, "device offline") || strings.Contains(detail, "device not found") {
		return apperror.Wrap("COMPANION_REINSTALL_DEVICE_OFFLINE", "重新安装伴侣时设备已离线", "companion.upgrade", true, cause).
			WithSuggestion("旧伴侣可能已卸载；请恢复无线调试连接后再次安装")
	}
	return apperror.Wrap("COMPANION_REINSTALL_FAILED", "旧伴侣已卸载，但新版伴侣安装失败", "companion.upgrade", true, cause).
		WithSuggestion(strings.TrimSpace(output.Stderr + " " + output.Stdout))
}

func (s *Service) logCompanionUpgrade(event, deviceID, trigger string, result CompanionUpgradeResult, duration time.Duration, err error) {
	if s.logger == nil {
		return
	}
	args := []any{
		"deviceId", deviceID,
		"trigger", trigger,
		"state", result.State,
		"previousVersionCode", result.PreviousVersionCode,
		"installedVersionCode", result.InstalledVersionCode,
		"requiredVersionCode", result.RequiredVersionCode,
		"updated", result.Updated,
		"durationMs", duration.Milliseconds(),
	}
	if err != nil {
		args = append(args, "error", err)
		s.logger.Error(event, args...)
		return
	}
	s.logger.Info(event, args...)
}
