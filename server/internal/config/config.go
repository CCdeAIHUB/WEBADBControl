package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Address           string
	CoreBinary        string
	CoreRemoteDataDir string
	CoreRemoteListen  string
	DataDir           string
	WebDir            string
	AuthToken         string
	CompanionAPK      string
	BehindProxy       bool
}

func Load() (Config, error) {
	dataDir := env("WEBADB_DATA_DIR", "./data")
	config := Config{
		Address:           env("WEBADB_ADDRESS", "127.0.0.1:8080"),
		CoreBinary:        env("WEBADB_CORE_BINARY", "./adbcontrol-core"),
		CoreRemoteDataDir: filepath.Join(dataDir, "remote"),
		CoreRemoteListen:  persistedRemoteListen(filepath.Join(dataDir, "settings.json")),
		DataDir:           dataDir,
		WebDir:            env("WEBADB_WEB_DIR", "./web"),
		AuthToken:         os.Getenv("WEBADB_AUTH_TOKEN"),
		CompanionAPK:      env("WEBADB_COMPANION_APK", "./companion.apk"),
		BehindProxy:       envBool("WEBADB_BEHIND_PROXY", false),
	}
	if _, _, err := net.SplitHostPort(config.Address); err != nil {
		return Config{}, fmt.Errorf("invalid WEBADB_ADDRESS: %w", err)
	}
	for _, path := range []string{config.DataDir, filepath.Join(config.DataDir, "uploads"), config.CoreRemoteDataDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return Config{}, err
		}
	}
	return config, nil
}

func persistedRemoteListen(path string) string {
	var value struct {
		RemoteEnabled bool   `json:"remoteEnabled"`
		RemoteAddress string `json:"remoteAddress"`
		RemotePort    int    `json:"remotePort"`
	}
	encoded, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(encoded, &value)
	}
	if !value.RemoteEnabled {
		return ""
	}
	if net.ParseIP(value.RemoteAddress) == nil {
		return ""
	}
	if value.RemotePort < 1024 || value.RemotePort > 65535 {
		return ""
	}
	return net.JoinHostPort(value.RemoteAddress, strconv.Itoa(value.RemotePort))
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}
