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
	CompanionUpgradeReady     = "ready"
	CompanionUpgradeInstalled = "installed"
	CompanionUpgradeUpdated   = "updated"
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
	output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "dumpsys", "package", companionPackageName))
	if err != nil {
		return CompanionPackageInfo{}, err
	}
	return parseCompanionPackageInfo(output.Stdout), nil
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
	return s.ensureCompanion(ctx, deviceID, trigger, false)
}

func (s *Service) ForceInstallConfiguredCompanion(ctx context.Context, deviceID, trigger string) (CompanionUpgradeResult, error) {
	return s.ensureCompanion(ctx, deviceID, trigger, true)
}

func (s *Service) ensureCompanion(ctx context.Context, deviceID, trigger string, force bool) (CompanionUpgradeResult, error) {
	requirement := s.companionRequirement
	if !requirement.configured() {
		return CompanionUpgradeResult{State: CompanionUpgradeReady}, nil
	}

	// Capabilities, permissions and screen startup may arrive concurrently from the UI.
	// Serialize per device and always probe again under the lock so only one install -r runs.
	lockValue, _ := s.companionUpgradeLocks.LoadOrStore(deviceID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	started := time.Now()
	current, err := s.ProbeCompanionPackage(ctx, deviceID)
	if err != nil {
		return CompanionUpgradeResult{}, err
	}
	result := CompanionUpgradeResult{
		State:                CompanionUpgradeReady,
		PreviousVersionCode:  current.VersionCode,
		PreviousVersionName:  current.VersionName,
		InstalledVersionCode: current.VersionCode,
		InstalledVersionName: current.VersionName,
		RequiredVersionCode:  requirement.VersionCode,
		RequiredVersionName:  requirement.VersionName,
	}
	if !force && current.Installed && current.VersionCode >= requirement.VersionCode {
		s.logCompanionUpgrade("companion_requirement_satisfied", deviceID, trigger, result, time.Since(started), nil)
		return result, nil
	}
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
		return apperror.Wrap("COMPANION_SIGNATURE_MISMATCH", "设备上的伴侣签名与服务端安装包不一致", "companion.upgrade", false, cause).
			WithSuggestion("为保护伴侣数据，系统不会自动卸载；请确认使用同一正式签名的 APK")
	case strings.Contains(detail, "device offline") || strings.Contains(detail, "device not found"):
		return apperror.Wrap("COMPANION_UPGRADE_DEVICE_OFFLINE", "伴侣升级时设备已离线", "companion.upgrade", true, cause).
			WithSuggestion("请保持无线调试页面开启并重新连接后重试")
	default:
		return apperror.Wrap("COMPANION_UPGRADE_FAILED", "伴侣覆盖安装失败", "companion.upgrade", true, cause).
			WithSuggestion(strings.TrimSpace(output.Stderr + " " + output.Stdout))
	}
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
