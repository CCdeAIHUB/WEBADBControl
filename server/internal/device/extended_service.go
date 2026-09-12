package device

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

// WirelessPairingQR intentionally exposes only the renderable QR and opaque session id.
// The pairing password never leaves Rust Core memory.
type WirelessPairingQR struct {
	SessionID   string `json:"sessionId"`
	ServiceName string `json:"serviceName"`
	QRSVG       string `json:"qrSvg"`
	MimeType    string `json:"mimeType"`
	ExpiresAt   uint64 `json:"expiresAt"`
}

type WirelessQRPairingResult struct {
	Paired      bool   `json:"paired"`
	ServiceName string `json:"serviceName"`
	Endpoint    string `json:"endpoint"`
}

func (s *Service) CreateWirelessPairingQR(ctx context.Context) (WirelessPairingQR, error) {
	var result WirelessPairingQR
	if err := s.core.Call(ctx, "adb.wifi.qr.create", map[string]any{}, &result); err != nil {
		return WirelessPairingQR{}, err
	}
	return result, nil
}

func (s *Service) PairWirelessQR(ctx context.Context, sessionID string) (WirelessQRPairingResult, error) {
	if strings.TrimSpace(sessionID) == "" {
		return WirelessQRPairingResult{}, apperror.New("ADB_QR_PAIRING_SESSION_INVALID", "二维码配对会话无效", "adb.wifi.qr", true)
	}
	var result WirelessQRPairingResult
	if err := s.core.Call(ctx, "adb.wifi.qr.pair", map[string]any{"sessionId": sessionID}, &result); err != nil {
		return WirelessQRPairingResult{}, err
	}
	return result, nil
}

func (s *Service) CancelWirelessQR(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return apperror.New("ADB_QR_PAIRING_SESSION_INVALID", "二维码配对会话无效", "adb.wifi.qr", true)
	}
	return s.core.Call(ctx, "adb.wifi.qr.cancel", map[string]any{"sessionId": sessionID}, nil)
}

func (s *Service) Pair(ctx context.Context, endpoint, code string) error {
	normalizedEndpoint, err := normalizeWirelessEndpoint(endpoint)
	if err != nil {
		return err
	}
	normalizedCode, err := normalizePairingCode(code)
	if err != nil {
		return err
	}
	_, err = s.Exec(ctx, []string{"pair", normalizedEndpoint, normalizedCode})
	return err
}

func (s *Service) Disconnect(ctx context.Context, deviceID string) error {
	_, err := s.Exec(ctx, []string{"disconnect", deviceID})
	return err
}

type RemovalResult struct {
	Disconnected []string `json:"disconnected"`
}

func (s *Service) Remove(ctx context.Context, deviceID string) (RemovalResult, error) {
	devices, err := s.List(ctx)
	if err != nil {
		return RemovalResult{}, err
	}
	connectionIDs := []string{deviceID}
	for _, candidate := range devices {
		if candidate.ID == deviceID || containsDeviceID(candidate.Aliases, deviceID) {
			connectionIDs = appendUnique([]string{candidate.ID}, candidate.Aliases...)
			break
		}
	}
	result := RemovalResult{}
	for _, connectionID := range appendUnique(nil, connectionIDs...) {
		if err := s.Disconnect(ctx, connectionID); err != nil {
			return result, err
		}
		result.Disconnected = append(result.Disconnected, connectionID)
	}
	return result, nil
}

func containsDeviceID(deviceIDs []string, target string) bool {
	for _, deviceID := range deviceIDs {
		if deviceID == target {
			return true
		}
	}
	return false
}

func (s *Service) EnableTCPIP(ctx context.Context, deviceID string, port int) error {
	if port < 1024 || port > 65535 {
		return apperror.New("ADB_TCPIP_PORT_INVALID", "TCP/IP 端口必须在 1024 到 65535 之间", "device.connection", true)
	}
	_, err := s.Exec(ctx, DeviceArgs(deviceID, "tcpip", strconv.Itoa(port)))
	return err
}

func (s *Service) KeepAlive(ctx context.Context, deviceID string) error {
	if !strings.Contains(deviceID, ":") {
		return apperror.New("ADB_KEEPALIVE_NOT_WIRELESS", "当前设备不是无线 ADB 连接，无需执行保活", "device.connection", true)
	}
	normalizedEndpoint, err := normalizeWirelessEndpoint(deviceID)
	if err != nil {
		return err
	}
	if _, err := s.Exec(ctx, []string{"connect", normalizedEndpoint}); err != nil {
		return err
	}
	_, err = s.Exec(ctx, DeviceArgs(normalizedEndpoint, "shell", "true"))
	return err
}

func (s *Service) Discover(ctx context.Context) ([]DiscoveredService, error) {
	output, err := s.Exec(ctx, []string{"mdns", "services"})
	if err != nil {
		return nil, err
	}
	return ParseMDNS(output.Stdout), nil
}

func (s *Service) LockState(ctx context.Context, deviceID string) (LockState, error) {
	output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "dumpsys window policy; dumpsys power"))
	if err != nil {
		return LockState{}, err
	}
	return ParseLockState(output.Stdout), nil
}

func (s *Service) Unlock(ctx context.Context, deviceID, pin string) error {
	if pin != "" {
		if matched, _ := regexp.MatchString(`^\d{4,16}$`, pin); !matched {
			return apperror.New("DEVICE_PIN_INVALID", "PIN 必须是 4 到 16 位数字", "device.lock", true)
		}
	}
	if _, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "input", "keyevent", "WAKEUP")); err != nil {
		return err
	}
	sizeOutput, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "wm", "size"))
	if err != nil {
		return err
	}
	width, height, err := parseScreenSize(sizeOutput.Stdout)
	if err != nil {
		return apperror.Wrap("DEVICE_SCREEN_SIZE_UNKNOWN", "无法确定解锁手势坐标", "device.lock", true, err)
	}
	if _, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "input", "swipe", strconv.Itoa(width/2), strconv.Itoa(height*3/4), strconv.Itoa(width/2), strconv.Itoa(height/4), "350")); err != nil {
		return err
	}
	if pin != "" {
		if _, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "input", "text", pin)); err != nil {
			return err
		}
		if _, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", "input", "keyevent", "ENTER")); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Hardware(ctx context.Context, deviceID string) (HardwareSnapshot, error) {
	command := `echo CPU=$(cat /sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq 2>/dev/null | tr '\n' ','); ` +
		`echo MEM=$(grep -E '^(MemTotal|MemAvailable):' /proc/meminfo | tr '\n' '|'); ` +
		`echo DISK=$(df -k /data 2>/dev/null | tail -n 1); ` +
		`echo TEMP=$(for z in /sys/class/thermal/thermal_zone*; do echo "$(cat $z/type 2>/dev/null):$(cat $z/temp 2>/dev/null)"; done | tr '\n' ','); ` +
		`echo UPTIME=$(cat /proc/uptime)`
	output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", command))
	if err != nil {
		return HardwareSnapshot{}, err
	}
	return ParseHardwareSnapshot(output.Stdout), nil
}
