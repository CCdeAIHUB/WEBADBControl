package device

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

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
	return s.pairWirelessQR(ctx, sessionID, 600*time.Millisecond)
}

func (s *Service) pairWirelessQR(ctx context.Context, sessionID string, retryInterval time.Duration) (WirelessQRPairingResult, error) {
	if strings.TrimSpace(sessionID) == "" {
		return WirelessQRPairingResult{}, apperror.New("ADB_QR_PAIRING_SESSION_INVALID", "二维码配对会话无效", "adb.wifi.qr", true)
	}
	for {
		var result WirelessQRPairingResult
		err := s.core.Call(ctx, "adb.wifi.qr.pair", map[string]any{"sessionId": sessionID}, &result)
		if err == nil {
			return result, nil
		}
		var appErr *apperror.Error
		if !errors.As(err, &appErr) || appErr.ErrorCode != "ADB_QR_PAIRING_NOT_DISCOVERED" {
			return WirelessQRPairingResult{}, err
		}
		// Android publishes the QR mDNS service only after the scanner accepts
		// the code. Retry this one recoverable state until the HTTP deadline or
		// client cancellation; every other Core error remains immediately visible.
		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return WirelessQRPairingResult{}, apperror.Wrap("ADB_QR_PAIRING_WAIT_CANCELLED", "等待手机广播二维码配对服务已结束", "adb.wifi.qr", true, ctx.Err())
		case <-timer.C:
		}
	}
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
	Forgotten    []string `json:"forgotten"`
}

func (s *Service) Remove(ctx context.Context, deviceID string) (RemovalResult, error) {
	liveDevices, err := s.listLive(ctx)
	if err != nil {
		return RemovalResult{}, err
	}
	connectionIDs := []string{}
	for _, candidate := range liveDevices {
		if !sameCatalogDevice(candidate, Device{ID: deviceID}) {
			continue
		}
		for _, connectionID := range append([]string{candidate.ID}, candidate.Aliases...) {
			if isWirelessADBIdentity(connectionID) {
				connectionIDs = appendUnique(connectionIDs, connectionID)
			}
		}
	}
	result := RemovalResult{}
	for _, connectionID := range connectionIDs {
		if err := s.Disconnect(ctx, connectionID); err != nil {
			return result, err
		}
		result.Disconnected = append(result.Disconnected, connectionID)
	}
	removed, found, err := s.catalog.Remove(deviceID)
	if err != nil {
		return result, apperror.Wrap("DEVICE_CATALOG_SAVE_FAILED", "设备记忆目录保存失败", "device.catalog", false, err)
	}
	if found {
		result.Forgotten = appendUnique([]string{removed.ID}, removed.Aliases...)
	} else {
		result.Forgotten = []string{deviceID}
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
	command := `echo brand=$(getprop ro.product.brand); echo model=$(getprop ro.product.model); echo device=$(getprop ro.product.device); echo android=$(getprop ro.build.version.release); echo sdk=$(getprop ro.build.version.sdk); echo abi=$(getprop ro.product.cpu.abi); ` +
		`cpu_model=$(getprop ro.soc.model); [ -n "$cpu_model" ] || cpu_model=$(getprop ro.board.platform); echo cpu_model="$cpu_model"; echo cpu_cores=$(getconf _NPROCESSORS_ONLN 2>/dev/null); ` +
		`echo cpu_freqs="$(for p in /sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq; do [ -r "$p" ] && printf '%s:%s,' "$(basename $(dirname $(dirname $p)))" "$(cat $p)"; done)"; ` +
		`echo cpu_max_freqs="$(for p in /sys/devices/system/cpu/cpu[0-9]*/cpufreq/cpuinfo_max_freq; do [ -r "$p" ] && printf '%s:%s,' "$(basename $(dirname $(dirname $p)))" "$(cat $p)"; done)"; ` +
		`battery_dump="$(dumpsys battery)"; echo battery_level=$(printf '%s\n' "$battery_dump" | grep -m1 'level:' | cut -d: -f2-); echo battery_status=$(printf '%s\n' "$battery_dump" | grep -m1 'status:' | cut -d: -f2-); echo battery_temp=$(printf '%s\n' "$battery_dump" | grep -m1 'temperature:' | cut -d: -f2-); ` +
		`echo mem_total_kb=$(awk '/MemTotal/ { print $2; exit }' /proc/meminfo); echo mem_available_kb=$(awk '/MemAvailable/ { print $2; exit }' /proc/meminfo); echo swap_total_kb=$(awk '/SwapTotal/ { print $2; exit }' /proc/meminfo); echo swap_free_kb=$(awk '/SwapFree/ { print $2; exit }' /proc/meminfo); echo zram_disk_bytes=$(cat /sys/block/zram0/disksize 2>/dev/null); echo load=$(cut -d' ' -f1-3 /proc/loadavg); ` +
		`echo CPU=$(cat /sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq 2>/dev/null | tr '\n' ','); ` +
		`echo MEM=$(grep -E '^(MemTotal|MemAvailable):' /proc/meminfo | tr '\n' '|'); ` +
		`echo DISK=$(df -k /data 2>/dev/null | tail -n 1); ` +
		`echo TEMP=$(for z in /sys/class/thermal/thermal_zone*; do echo "$(cat $z/type 2>/dev/null):$(cat $z/temp 2>/dev/null)"; done | tr '\n' ','); ` +
		`echo UPTIME=$(cat /proc/uptime); ` +
		`gpu_path=$(find /sys/class/devfreq /sys/class/kgsl -maxdepth 2 -type f \( -name cur_freq -o -name gpuclk \) 2>/dev/null | head -n1); gpu_dir=$(dirname "$gpu_path"); [ -n "$gpu_path" ] && gpu_access=available || gpu_access=unsupported; echo gpu_access=$gpu_access; echo gpu_cur_freq=$(cat "$gpu_path" 2>/dev/null); echo gpu_max_freq=$(cat "$gpu_dir/max_freq" 2>/dev/null); echo gpu_usage=$(cat "$gpu_dir/load" 2>/dev/null | grep -o -E '[0-9]+([.][0-9]+)?' | head -n1); echo gpu_memory_bytes=$(dumpsys gpu 2>/dev/null | grep -m1 '^Global total:' | grep -o -E '[0-9]+' | head -n1); ` +
		`display_dump="$(dumpsys display 2>/dev/null)"; echo refresh_rate=$(printf '%s\n' "$display_dump" | grep -m1 -E 'mRefreshRate|refreshRate' | grep -o -E '[0-9]+(\.[0-9]+)?' | head -n1)`
	output, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", command))
	if err != nil {
		return HardwareSnapshot{}, err
	}
	return ParseHardwareSnapshot(output.Stdout), nil
}
