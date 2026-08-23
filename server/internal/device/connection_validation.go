package device

import (
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

var pairingCodePattern = regexp.MustCompile(`^\d{6}$`)
var hostNamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

func normalizeWirelessEndpoint(endpoint string) (string, error) {
	normalized := strings.TrimSpace(endpoint)
	host, rawPort, err := net.SplitHostPort(normalized)
	if err != nil || host == "" || strings.ContainsAny(host, " \t\r\n\x00") {
		return "", apperror.New("ADB_ENDPOINT_INVALID", "无线设备地址必须包含有效主机和端口", "device.connection", true).
			WithSuggestion("请使用 Android 无线调试页面显示的地址，例如 192.168.1.20:37123")
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return "", apperror.New("ADB_ENDPOINT_INVALID", "无线设备端口无效", "device.connection", true).
			WithSuggestion("请重新复制无线调试页面显示的完整地址和端口")
	}
	if net.ParseIP(host) == nil && !hostNamePattern.MatchString(host) {
		return "", apperror.New("ADB_ENDPOINT_INVALID", "无线设备主机名无效", "device.connection", true)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func normalizePairingCode(code string) (string, error) {
	normalized := strings.TrimSpace(code)
	if !pairingCodePattern.MatchString(normalized) {
		return "", apperror.New("ADB_PAIR_CODE_INVALID", "无线配对码必须是 6 位数字", "device.connection", true).
			WithSuggestion("请在 Android 的“使用配对码配对设备”窗口中重新获取六位码")
	}
	return normalized, nil
}
