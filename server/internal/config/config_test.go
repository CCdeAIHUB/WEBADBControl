package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistedRemoteListenIsDisabledByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if got := persistedRemoteListen(path); got != "" {
		t.Fatalf("missing settings address = %q", got)
	}
	if err := os.WriteFile(path, []byte(`{"remoteEnabled":false,"remoteAddress":"0.0.0.0","remotePort":9000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := persistedRemoteListen(path); got != "" {
		t.Fatalf("disabled remote address = %q", got)
	}
}

func TestPersistedRemoteListenUsesValidatedCoreQUICEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"remoteEnabled":true,"remoteAddress":"0.0.0.0","remotePort":9000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := persistedRemoteListen(path); got != "0.0.0.0:9000" {
		t.Fatalf("enabled remote address = %q", got)
	}
}

func TestLoadKeepsWebHTTPAndCoreQUICListenersIndependent(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dataDir, "settings.json"),
		[]byte(`{"remoteEnabled":true,"remoteAddress":"0.0.0.0","remotePort":45921}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBADB_DATA_DIR", dataDir)
	t.Setenv("WEBADB_ADDRESS", "")

	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.Address != "127.0.0.1:8080" {
		t.Fatalf("Web HTTP address = %q", config.Address)
	}
	if config.CoreRemoteListen != "0.0.0.0:45921" {
		t.Fatalf("Core QUIC address = %q", config.CoreRemoteListen)
	}
}

func TestLoadUsesConfiguredCompanionCapabilityBaseline(t *testing.T) {
	// 场景：发布新伴侣后，部署环境可以提升能力基线，而不需要修改业务代码。
	dataDir := t.TempDir()
	t.Setenv("WEBADB_DATA_DIR", dataDir)
	t.Setenv("WEBADB_COMPANION_VERSION_CODE", "17")
	t.Setenv("WEBADB_COMPANION_VERSION_NAME", "0.17.0")

	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.CompanionVersionCode != 17 || config.CompanionVersionName != "0.17.0" {
		t.Fatalf("companion baseline = %d/%q", config.CompanionVersionCode, config.CompanionVersionName)
	}
}

func TestLoadRejectsInvalidCompanionVersionCode(t *testing.T) {
	// 场景：无效环境变量不能关闭升级保护，应回退到随服务发布的版本基线。
	t.Setenv("WEBADB_DATA_DIR", t.TempDir())
	t.Setenv("WEBADB_COMPANION_VERSION_CODE", "invalid")

	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.CompanionVersionCode != 13 {
		t.Fatalf("companion version code = %d, want 13", config.CompanionVersionCode)
	}
}
