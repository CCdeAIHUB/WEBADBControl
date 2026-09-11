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
