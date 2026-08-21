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
	ID        string `json:"id"`
	Name      string `json:"name"`
	Model     string `json:"model,omitempty"`
	Product   string `json:"product,omitempty"`
	State     string `json:"state"`
	Transport string `json:"transport"`
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
		return output, apperror.New("ADB_COMMAND_FAILED", "设备命令执行失败", "device.service", true).
			WithSuggestion(strings.TrimSpace(output.Stderr))
	}
	return output, nil
}

func (s *Service) List(ctx context.Context) ([]Device, error) {
	output, err := s.Exec(ctx, []string{"devices", "-l"})
	if err != nil {
		return nil, err
	}
	return parseDevices(output.Stdout), nil
}

func parseDevices(output string) []Device {
	devices := make([]Device, 0)
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "List" {
			continue
		}
		device := Device{ID: fields[0], Name: fields[0], State: fields[1], Transport: "usb"}
		if strings.Contains(device.ID, ":") {
			device.Transport = "wireless"
		}
		for _, field := range fields[2:] {
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

func (s *Service) Connect(ctx context.Context, endpoint string) error {
	if strings.TrimSpace(endpoint) == "" || strings.ContainsAny(endpoint, "\r\n\x00") {
		return apperror.New("ADB_ENDPOINT_INVALID", "无线设备地址无效", "device.connection", true)
	}
	if _, err := s.Exec(ctx, []string{"connect", endpoint}); err != nil {
		return err
	}
	devices, err := s.List(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range devices {
		if candidate.ID == endpoint && candidate.State == "device" {
			return nil
		}
	}
	return apperror.New("ADB_CONNECT_NOT_VERIFIED", "ADB 未确认设备在线", "device.connection", true)
}

func (s *Service) Action(ctx context.Context, deviceID string, request ActionRequest) error {
	args, err := ActionArguments(deviceID, request)
	if err != nil {
		return apperror.Wrap("DEVICE_ACTION_INVALID", "设备操作参数无效", "device.control", false, err)
	}
	_, err = s.Exec(ctx, args)
	return err
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
	battery, _ := s.Exec(ctx, []string{"-s", deviceID, "shell", "dumpsys", "battery"})
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
