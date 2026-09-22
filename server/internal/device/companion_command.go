package device

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

const (
	companionPackageName           = "com.adbcontrol.companion"
	companionCommandAction         = "com.adbcontrol.companion.EXECUTE_COMMAND"
	companionCommandResultPath     = "/sdcard/Android/data/com.adbcontrol.companion/files/command-results"
	CompanionTransportQUIC         = "quic"
	CompanionTransportADBBroadcast = "adb-broadcast"
)

type CompanionCollectionAccess struct {
	Items     []map[string]any
	Transport string
}

type CompanionStatus struct {
	Installed     bool   `json:"installed"`
	ADBResponsive bool   `json:"adbResponsive"`
	State         string `json:"state"`
	Message       string `json:"message"`
	ErrorCode     string `json:"errorCode,omitempty"`
}

func (s *Service) ExecuteCompanionCommand(ctx context.Context, deviceID, capabilityID, operation string, args map[string]any) (string, error) {
	if strings.TrimSpace(capabilityID) == "" || strings.TrimSpace(operation) == "" {
		return "", apperror.New("COMPANION_COMMAND_INVALID", "Companion 能力或操作为空", "device.companion", false)
	}
	requestID := randomRequestID()
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		return "", apperror.Wrap("COMPANION_ARGS_INVALID", "Companion 参数无法编码", "device.companion", false, err)
	}
	command := "am broadcast --receiver-foreground" +
		" -a " + shellQuote(companionCommandAction) +
		" -n " + shellQuote(companionPackageName+"/.commands.CompanionCommandReceiver") +
		" --es requestId " + shellQuote(requestID) +
		" --es capabilityId " + shellQuote(capabilityID) +
		" --es operation " + shellQuote(operation) +
		" --es argsJson " + shellQuote(string(encodedArgs))
	if _, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", command)); err != nil {
		return "", err
	}
	payload, err := s.readCompanionCommandResult(ctx, deviceID, requestID)
	if err != nil {
		return "", err
	}
	if err := validateCompanionCommandResult(payload); err != nil {
		return "", err
	}
	return payload, nil
}

func (s *Service) CompanionCapabilities(ctx context.Context, deviceID string) (CompanionCollectionAccess, error) {
	var capabilities []map[string]any
	err := s.core.Call(ctx, "device.getCapabilities", map[string]any{"deviceId": deviceID}, &capabilities)
	if err == nil {
		return CompanionCollectionAccess{Items: capabilities, Transport: CompanionTransportQUIC}, nil
	}
	if !isCompanionSessionUnavailable(err) {
		return CompanionCollectionAccess{}, err
	}

	// The static catalog remains owned by Rust Core. ADB broadcast is the same
	// compatibility path used by the Windows client while QUIC is not ready.
	if err := s.core.Call(ctx, "capability.list", map[string]any{}, &capabilities); err != nil {
		return CompanionCollectionAccess{}, err
	}
	return CompanionCollectionAccess{Items: capabilities, Transport: CompanionTransportADBBroadcast}, nil
}

func (s *Service) CompanionPermissions(ctx context.Context, deviceID string) (CompanionCollectionAccess, error) {
	var permissions []map[string]any
	err := s.core.Call(ctx, "device.getPermissionState", map[string]any{"deviceId": deviceID}, &permissions)
	if err == nil {
		return CompanionCollectionAccess{Items: permissions, Transport: CompanionTransportQUIC}, nil
	}
	if !isCompanionSessionUnavailable(err) {
		return CompanionCollectionAccess{}, err
	}
	// ADB broadcast validates permissions per operation and returns a structured
	// permission error. It cannot publish the full QUIC permission matrix.
	return CompanionCollectionAccess{Items: []map[string]any{}, Transport: CompanionTransportADBBroadcast}, nil
}

func (s *Service) InvokeCompanionCapability(ctx context.Context, deviceID, capabilityID, operation string, args map[string]any) (map[string]any, string, error) {
	var result map[string]any
	err := s.core.Call(ctx, "device.invoke", map[string]any{
		"deviceId": deviceID, "capabilityId": capabilityID, "operation": operation, "args": args,
	}, &result)
	if err == nil {
		return result, CompanionTransportQUIC, nil
	}
	if !isCompanionSessionUnavailable(err) {
		return nil, CompanionTransportQUIC, err
	}

	payload, err := s.ExecuteCompanionCommand(ctx, deviceID, capabilityID, operation, args)
	if err != nil {
		return nil, CompanionTransportADBBroadcast, err
	}
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return nil, CompanionTransportADBBroadcast, apperror.Wrap("COMPANION_RESULT_INVALID", "Companion 返回了无法解析的结果", "device.companion", true, err)
	}
	return result, CompanionTransportADBBroadcast, nil
}

type companionCommandEnvelope struct {
	OK    bool            `json:"ok"`
	Error *apperror.Error `json:"error,omitempty"`
}

func validateCompanionCommandResult(payload string) error {
	var result companionCommandEnvelope
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return apperror.Wrap("COMPANION_RESULT_INVALID", "Companion 返回了无法解析的结果", "device.companion", true, err)
	}
	if result.OK {
		return nil
	}
	if result.Error != nil && result.Error.ErrorCode != "" {
		if result.Error.TraceID == "" {
			result.Error.TraceID = randomRequestID()
		}
		return result.Error
	}
	return apperror.New("COMPANION_COMMAND_FAILED", "Companion 命令执行失败", "device.companion", true)
}

func (s *Service) CompanionStatus(ctx context.Context, deviceID string) (CompanionStatus, error) {
	output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "pm", "list", "packages", companionPackageName))
	if err != nil {
		return CompanionStatus{}, err
	}
	if !strings.Contains(output.Stdout, "package:"+companionPackageName) {
		return CompanionStatus{
			Installed: false,
			State:     "missing",
			Message:   "设备未安装 ADBControl Companion。",
		}, nil
	}

	// `accessibility.status` is the Companion's side-effect-free readiness probe:
	// it only reports service readiness and must not open an Activity or Android
	// settings page. This is important for broken-screen/external-display devices
	// where repeated foreground launches can interrupt control.
	_, err = s.ExecuteCompanionCommand(ctx, deviceID, "android.accessibility.control", "accessibility.status", map[string]any{})
	if err != nil {
		return CompanionStatus{
			Installed:     true,
			ADBResponsive: false,
			State:         "adb-unreachable",
			Message:       "伴侣应用已安装，但 ADB broadcast 探测未返回结果。",
			ErrorCode:     companionErrorCode(err),
		}, nil
	}
	return CompanionStatus{
		Installed:     true,
		ADBResponsive: true,
		State:         "adb-responsive",
		Message:       "伴侣应用已安装，且 ADB broadcast 探测可达。",
	}, nil
}

func (s *Service) readCompanionCommandResult(ctx context.Context, deviceID, requestID string) (string, error) {
	resultPath := companionCommandResultPath + "/" + requestID + ".json"
	var lastErr error
	// Cold-starting a stopped Companion process on OEM Android can take more
	// than the old 800ms window, especially over wireless ADB.
	for attempt := 0; attempt < 30; attempt++ {
		output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "cat", resultPath))
		if err == nil && strings.TrimSpace(output.Stdout) != "" {
			return strings.TrimSpace(output.Stdout), nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", apperror.New("COMPANION_RESULT_MISSING", "Companion 未返回命令结果", "device.companion", true)
}

func isCompanionSessionUnavailable(err error) bool {
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		return false
	}
	return appErr.ErrorCode == "COMPANION_DEVICE_NOT_CONNECTED" || appErr.ErrorCode == "COMPANION_SESSION_NOT_CONNECTED"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func randomRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "web" + time.Now().UTC().Format("20060102150405")
	}
	return hex.EncodeToString(buffer)
}

func companionErrorCode(err error) string {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return appErr.ErrorCode
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "COMPANION_STATUS_TIMEOUT"
	}
	if errors.Is(err, context.Canceled) {
		return "COMPANION_STATUS_CANCELED"
	}
	return "COMPANION_STATUS_PROBE_FAILED"
}
