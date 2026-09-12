package device

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/coreipc"
)

type Device struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Model      string   `json:"model,omitempty"`
	Product    string   `json:"product,omitempty"`
	State      string   `json:"state"`
	Transport  string   `json:"transport"`
	HardwareID string   `json:"hardwareId,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
}

type CommandOutput struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

type Service struct{ core coreipc.Caller }

func NewService(core coreipc.Caller) *Service { return &Service{core: core} }

func (s *Service) Exec(ctx context.Context, args []string) (CommandOutput, error) {
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return CommandOutput{}, apperror.New("ADB_ARGS_CONTAIN_NUL", "ADB 参数包含非法字符", "device.service", false)
		}
	}
	var output CommandOutput
	if err := s.core.Call(ctx, "adb.exec", map[string]any{"args": args}, &output); err != nil {
		return CommandOutput{}, err
	}
	if output.ExitCode != 0 {
		return output, adbCommandFailure(args, output)
	}
	return output, nil
}

func (s *Service) List(ctx context.Context) ([]Device, error) {
	output, err := s.Exec(ctx, []string{"devices", "-l"})
	if err != nil {
		return nil, err
	}
	return s.attachIdentityAndDedupe(ctx, parseDevices(output.Stdout)), nil
}

func (s *Service) attachIdentityAndDedupe(ctx context.Context, devices []Device) []Device {
	if len(devices) <= 1 {
		return devices
	}
	identified := make([]Device, 0, len(devices))
	for _, candidate := range devices {
		device := candidate
		if candidate.State == "device" {
			if hardwareID, err := s.readHardwareID(ctx, candidate.ID); err == nil && hardwareID != "" {
				device.HardwareID = hardwareID
			}
		}
		if device.HardwareID == "" {
			device.HardwareID = fallbackHardwareID(device)
		}
		identified = append(identified, device)
	}
	return dedupeDevicesByHardwareID(identified)
}

func (s *Service) readHardwareID(ctx context.Context, deviceID string) (string, error) {
	// ADB may expose the same physical phone as USB and wireless transports.
	// A stable device-side identifier lets the UI keep one card per handset instead of one card per transport.
	output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "printf 'serial='; getprop ro.serialno; printf '\nbootserial='; getprop ro.boot.serialno; printf '\nandroid_id='; settings get secure android_id 2>/dev/null"))
	if err != nil {
		return "", err
	}
	return ParseHardwareID(output.Stdout), nil
}

func ParseHardwareID(output string) string {
	values := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	for _, key := range []string{"serial", "bootserial", "android_id"} {
		if value := sanitizeHardwareID(values[key]); value != "" {
			return key + ":" + value
		}
	}
	return ""
}

func sanitizeHardwareID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if lower == "unknown" || lower == "null" || lower == "none" || lower == "0" {
		return ""
	}
	return value
}

func fallbackHardwareID(device Device) string {
	if serviceID := normalizedMDNSServiceIdentity(device.ID); serviceID != "" {
		// ADB resolves a duplicate mDNS service name by appending " (n)" to the
		// serial. That suffix is a local collision marker, not a second handset.
		return "mdns:" + serviceID
	}
	if device.Model != "" && device.Product != "" && device.Transport == "wireless" {
		return "wireless:" + device.Product + ":" + device.Model
	}
	return "transport:" + device.ID
}

func dedupeDevicesByHardwareID(devices []Device) []Device {
	result := make([]Device, 0, len(devices))
	indexByHardwareID := map[string]int{}
	for _, candidate := range devices {
		key := candidate.HardwareID
		if key == "" {
			key = "transport:" + candidate.ID
		}
		if index, exists := indexByHardwareID[key]; exists {
			kept := result[index]
			kept.Aliases = appendUnique(kept.Aliases, candidate.ID)
			kept.Aliases = appendUnique(kept.Aliases, candidate.Aliases...)
			kept.Aliases = aliasesWithoutPrimary(kept.ID, kept.Aliases)
			if shouldPreferDevice(candidate, kept) {
				candidate.Aliases = appendUnique(append(candidate.Aliases, kept.ID), kept.Aliases...)
				// Aliases identify alternate transports only; retaining the selected primary ID
				// would make clients treat the same transport as both primary and fallback.
				candidate.Aliases = aliasesWithoutPrimary(candidate.ID, candidate.Aliases)
				result[index] = candidate
			} else {
				result[index] = kept
			}
			continue
		}
		indexByHardwareID[key] = len(result)
		result = append(result, candidate)
	}
	return result
}

func shouldPreferDevice(candidate, current Device) bool {
	if candidate.State == "device" && current.State != "device" {
		return true
	}
	if candidate.State != "device" && current.State == "device" {
		return false
	}
	if candidate.Transport == "usb" && current.Transport == "wireless" {
		return true
	}
	return false
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	result := make([]string, 0, len(values)+len(additions))
	for _, value := range append(values, additions...) {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func aliasesWithoutPrimary(primary string, aliases []string) []string {
	result := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if alias != primary {
			result = append(result, alias)
		}
	}
	return result
}

func parseDevices(output string) []Device {
	devices := make([]Device, 0)
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "List" {
			continue
		}
		stateIndex, stateWidth := adbDeviceStateIndex(fields)
		if stateIndex <= 0 {
			continue
		}
		deviceID := strings.Join(fields[:stateIndex], " ")
		device := Device{ID: deviceID, Name: deviceID, State: strings.Join(fields[stateIndex : stateIndex+stateWidth], " "), Transport: "usb"}
		if isWirelessADBIdentity(device.ID) {
			device.Transport = "wireless"
		}
		for _, field := range fields[stateIndex+stateWidth:] {
			key, value, ok := strings.Cut(field, ":")
			if !ok {
				continue
			}
			switch key {
			case "model":
				device.Model, device.Name = value, strings.ReplaceAll(value, "_", " ")
			case "product":
				device.Product = value
			}
		}
		devices = append(devices, device)
	}
	return devices
}

func adbDeviceStateIndex(fields []string) (int, int) {
	for index, field := range fields {
		switch field {
		case "device", "offline", "unauthorized":
			return index, 1
		case "no":
			if index+1 < len(fields) && fields[index+1] == "permissions" {
				return index, 2
			}
		}
	}
	return -1, 0
}

func isWirelessADBIdentity(deviceID string) bool {
	return strings.Contains(deviceID, ":") ||
		strings.Contains(deviceID, "._adb-tls-connect._tcp") ||
		strings.Contains(deviceID, "._adb._tcp")
}

func normalizedMDNSServiceIdentity(deviceID string) string {
	const suffix = "._adb-tls-connect._tcp"
	serviceName, found := strings.CutSuffix(deviceID, suffix)
	if !found {
		return ""
	}
	if opening := strings.LastIndex(serviceName, " ("); opening >= 0 && strings.HasSuffix(serviceName, ")") {
		instance := serviceName[opening+2 : len(serviceName)-1]
		if instance != "" {
			allDigits := true
			for _, character := range instance {
				if character < '0' || character > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				serviceName = serviceName[:opening]
			}
		}
	}
	return serviceName
}

func isOnlineEndpoint(candidate Device, endpoint string) bool {
	if candidate.State != "device" {
		return false
	}
	if candidate.ID == endpoint {
		return true
	}
	for _, alias := range candidate.Aliases {
		if alias == endpoint {
			return true
		}
	}
	return false
}

func (s *Service) Connect(ctx context.Context, endpoint string) error {
	normalizedEndpoint, err := normalizeWirelessEndpoint(endpoint)
	if err != nil {
		return err
	}
	if _, err := s.Exec(ctx, []string{"connect", normalizedEndpoint}); err != nil {
		return err
	}
	devices, err := s.List(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range devices {
		// Hardware-ID deduplication may retain the mDNS serial or USB serial as
		// primary and preserve the requested IP endpoint as an alias. Verification
		// must accept that alias, while still requiring ADB's online "device" state.
		if isOnlineEndpoint(candidate, normalizedEndpoint) {
			return nil
		}
	}
	return apperror.New("ADB_CONNECT_NOT_VERIFIED", "ADB 未确认设备在线", "device.connection", true).
		WithSuggestion("请确认设备使用的是无线调试页面中的连接端口，而不是六位码配对端口")
}

type ActionOutcome struct {
	Transport string `json:"transport"`
}

func (s *Service) Action(ctx context.Context, deviceID string, request ActionRequest) error {
	_, err := s.ActionWithOutcome(ctx, deviceID, request)
	return err
}

func (s *Service) ActionWithOutcome(ctx context.Context, deviceID string, request ActionRequest) (ActionOutcome, error) {
	args, err := ActionArguments(deviceID, request)
	if err != nil {
		return ActionOutcome{}, apperror.Wrap("DEVICE_ACTION_INVALID", "设备操作参数无效", "device.control", false, err)
	}
	_, err = s.Exec(ctx, args)
	if err == nil || !isInputInjectionDenied(err) {
		return ActionOutcome{Transport: "adb-input"}, err
	}
	transport, fallbackErr := s.fallbackDeniedInput(ctx, deviceID, request)
	return ActionOutcome{Transport: transport}, fallbackErr
}

func (s *Service) Screenshot(ctx context.Context, deviceID string) ([]byte, error) {
	// Device-side base64 keeps binary PNG bytes out of the Core UTF-8 JSON contract.
	output, err := s.Exec(ctx, []string{"-s", deviceID, "shell", "screencap -p | base64"})
	if err != nil {
		return nil, err
	}
	cleaned := strings.NewReplacer("\r", "", "\n", "", " ", "").Replace(output.Stdout)
	png, decodeErr := base64.StdEncoding.DecodeString(cleaned)
	if decodeErr != nil {
		return nil, apperror.Wrap("SCREENSHOT_INVALID", "设备截图无法解析", "device.screen", true, decodeErr)
	}
	return png, nil
}

func (s *Service) Overview(ctx context.Context, deviceID string) (map[string]any, error) {
	properties, err := s.Exec(ctx, []string{"-s", deviceID, "shell", "getprop"})
	if err != nil {
		return nil, err
	}
	battery, err := s.Exec(ctx, []string{"-s", deviceID, "shell", "dumpsys", "battery"})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"deviceId":    deviceID,
		"properties":  parseProperties(properties.Stdout),
		"battery":     parseKeyValueLines(battery.Stdout),
		"collectedAt": time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func parseProperties(output string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "]: [", 2)
		if len(parts) == 2 {
			result[strings.TrimPrefix(parts[0], "[")] = strings.TrimSuffix(parts[1], "]")
		}
	}
	return result
}

func parseKeyValueLines(output string) map[string]any {
	result := map[string]any{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if number, err := strconv.Atoi(value); err == nil {
			result[key] = number
		} else {
			result[key] = value
		}
	}
	return result
}

func (s *Service) Core() coreipc.Caller { return s.core }

func DeviceArgs(deviceID string, args ...string) []string {
	return append([]string{"-s", deviceID}, args...)
}

func ValidateRemotePath(path string) error {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n\x00") {
		return fmt.Errorf("remote path must be absolute")
	}
	return nil
}
