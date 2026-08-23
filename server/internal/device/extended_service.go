package device

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

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
