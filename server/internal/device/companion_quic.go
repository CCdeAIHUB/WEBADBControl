package device

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

const (
	companionConfigureAction    = "com.adbcontrol.companion.CONFIGURE_CONNECTION"
	companionConfigureReceiver  = companionPackageName + "/.quic.CompanionConnectionReceiver"
	companionProvisionCooldown  = 30 * time.Second
	companionConnectWaitTimeout = 4 * time.Second
)

type CompanionConnectionInfo struct {
	ListenAddress                string `json:"listenAddress"`
	ServerName                   string `json:"serverName"`
	CertificateDERBase64         string `json:"certificateDerBase64"`
	CertificateFingerprintSHA256 string `json:"certificateFingerprintSha256"`
}

type CompanionProvisionResult struct {
	Endpoint                     string `json:"endpoint"`
	ServerName                   string `json:"serverName"`
	CertificateFingerprintSHA256 string `json:"certificateFingerprintSha256"`
	ConfiguredAt                 string `json:"configuredAt"`
}

func (s *Service) ProvisionCompanionQUIC(ctx context.Context, deviceID string) (CompanionProvisionResult, error) {
	var info CompanionConnectionInfo
	if err := s.core.Call(ctx, "companion.connection.info", map[string]any{}, &info); err != nil {
		return CompanionProvisionResult{}, err
	}
	_, portText, err := net.SplitHostPort(info.ListenAddress)
	if err != nil {
		return CompanionProvisionResult{}, apperror.Wrap(
			"COMPANION_QUIC_LISTENER_INVALID",
			"Core 返回了无效的伴侣 QUIC 监听地址",
			"companion.provision",
			false,
			err,
		)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return CompanionProvisionResult{}, apperror.New(
			"COMPANION_QUIC_LISTENER_INVALID",
			"Core 返回了无效的伴侣 QUIC 端口",
			"companion.provision",
			false,
		)
	}
	if strings.TrimSpace(info.ServerName) == "" || strings.TrimSpace(info.CertificateDERBase64) == "" {
		return CompanionProvisionResult{}, apperror.New(
			"COMPANION_QUIC_IDENTITY_MISSING",
			"Core 未提供完整的伴侣 QUIC TLS 身份",
			"companion.provision",
			false,
		)
	}
	host, err := resolveCompanionPublicHost(s.companionPublicHost, deviceID, routeLocalAddress)
	if err != nil {
		return CompanionProvisionResult{}, err
	}
	endpoint := "quic://" + net.JoinHostPort(host, strconv.Itoa(port))
	command := "am broadcast --receiver-foreground" +
		" -a " + shellQuote(companionConfigureAction) +
		" -n " + shellQuote(companionConfigureReceiver) +
		" --es endpoint " + shellQuote(endpoint) +
		" --es deviceId " + shellQuote(deviceID) +
		" --es serverName " + shellQuote(info.ServerName) +
		" --es certificateDerBase64 " + shellQuote(info.CertificateDERBase64)
	if _, err := s.Exec(ctx, DeviceArgs(deviceID, "shell", command)); err != nil {
		return CompanionProvisionResult{}, apperror.Wrap(
			"COMPANION_QUIC_CONFIGURE_BROADCAST_FAILED",
			"无法通过 ADB 向伴侣下发 QUIC 连接配置",
			"companion.provision",
			true,
			err,
		).WithSuggestion("请确认设备仍在线、伴侣已安装，并允许 Core 的 UDP 伴侣端口通过防火墙")
	}
	result := CompanionProvisionResult{
		Endpoint:                     endpoint,
		ServerName:                   info.ServerName,
		CertificateFingerprintSHA256: info.CertificateFingerprintSHA256,
		ConfiguredAt:                 time.Now().UTC().Format(time.RFC3339),
	}
	s.companionProvisioned.Store(deviceID, time.Now())
	if s.logger != nil {
		s.logger.Info(
			"companion_quic_configuration_sent",
			"deviceId", deviceID,
			"endpoint", endpoint,
			"certificateFingerprintPrefix", fingerprintPrefix(info.CertificateFingerprintSHA256),
		)
	}
	return result, nil
}

func (s *Service) provisionCompanionQUICIfDue(ctx context.Context, deviceID string) (bool, error) {
	if configuredAt, ok := s.companionProvisioned.Load(deviceID); ok {
		if timestamp, valid := configuredAt.(time.Time); valid && time.Since(timestamp) < companionProvisionCooldown {
			return false, nil
		}
	}
	_, err := s.ProvisionCompanionQUIC(ctx, deviceID)
	return err == nil, err
}

func (s *Service) connectCompanionQUICIfPossible(ctx context.Context, deviceID string) bool {
	configured, err := s.provisionCompanionQUICIfDue(ctx, deviceID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("companion_quic_provision_failed", "deviceId", deviceID, "error", err)
		}
		return false
	}
	return configured && s.waitForCompanionQUIC(ctx, deviceID)
}

func (s *Service) waitForCompanionQUIC(ctx context.Context, deviceID string) bool {
	deadline := time.NewTimer(companionConnectWaitTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		var capabilities []map[string]any
		if err := s.core.Call(ctx, "device.getCapabilities", map[string]any{"deviceId": deviceID}, &capabilities); err == nil {
			return true
		} else if !isCompanionSessionUnavailable(err) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}

func resolveCompanionPublicHost(
	configuredHost string,
	deviceID string,
	routeResolver func(string) (string, error),
) (string, error) {
	if host := strings.TrimSpace(configuredHost); host != "" {
		if net.ParseIP(host) == nil || net.ParseIP(host).IsUnspecified() {
			return "", apperror.New(
				"COMPANION_QUIC_PUBLIC_HOST_INVALID",
				"配置的伴侣 QUIC 局域网地址无效",
				"companion.provision",
				false,
			)
		}
		return host, nil
	}
	deviceHost, _, err := net.SplitHostPort(strings.TrimSpace(deviceID))
	if err != nil || net.ParseIP(deviceHost) == nil {
		return "", apperror.New(
			"COMPANION_QUIC_PUBLIC_HOST_UNAVAILABLE",
			"无法自动确定手机可访问的 Core 局域网地址",
			"companion.provision",
			true,
		).WithSuggestion("请设置 WEBADB_COMPANION_PUBLIC_HOST 为路由器或服务器的局域网 IP")
	}
	host, err := routeResolver(deviceHost)
	if err != nil || net.ParseIP(host) == nil || net.ParseIP(host).IsUnspecified() {
		if err == nil {
			err = fmt.Errorf("route resolver returned invalid local address %q", host)
		}
		return "", apperror.Wrap(
			"COMPANION_QUIC_PUBLIC_HOST_UNAVAILABLE",
			"无法根据无线设备路由确定 Core 局域网地址",
			"companion.provision",
			true,
			err,
		).WithSuggestion("请设置 WEBADB_COMPANION_PUBLIC_HOST 为手机能够访问的局域网 IP")
	}
	return host, nil
}

func routeLocalAddress(remoteHost string) (string, error) {
	connection, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP(remoteHost), Port: 9})
	if err != nil {
		return "", err
	}
	defer connection.Close()
	local, ok := connection.LocalAddr().(*net.UDPAddr)
	if !ok || local.IP == nil {
		return "", fmt.Errorf("UDP route did not expose a local IP")
	}
	return local.IP.String(), nil
}

func fingerprintPrefix(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
