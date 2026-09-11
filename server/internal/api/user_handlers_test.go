package api

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/config"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
)

type accountDeviceCaller struct{}

func (accountDeviceCaller) Call(_ context.Context, _ string, _ any, target any) error {
	*(target.(*device.CommandOutput)) = device.CommandOutput{Stdout: "List of devices attached\ndevice-1 device product:test model:Phone\n"}
	return nil
}

func TestAdminAccountManagementAndUserAuthorization(t *testing.T) {
	const adminPassword = "secure-admin-password"
	manager, core := newTestAuthManager()
	core.password = adminPassword
	server := &Server{
		config:  config.Config{},
		auth:    manager,
		devices: device.NewService(accountDeviceCaller{}),
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	handler := server.Handler()
	adminLogin := requestJSON(t, handler, http.MethodPost, "/api/v1/session", map[string]string{"username": "admin", "password": adminPassword}, nil)
	adminCookie := adminLogin.Result().Cookies()[0]

	created := requestJSON(t, handler, http.MethodPost, "/api/v1/users", map[string]string{"username": "operator", "password": "operator-password"}, adminCookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("create user status = %d body=%s", created.Code, created.Body.String())
	}
	unknown := requestJSON(t, handler, http.MethodPut, "/api/v1/users/operator/devices", map[string]any{"deviceId": "missing", "assigned": true}, adminCookie)
	if unknown.Code != http.StatusConflict {
		t.Fatalf("unknown device status = %d body=%s", unknown.Code, unknown.Body.String())
	}
	assigned := requestJSON(t, handler, http.MethodPut, "/api/v1/users/operator/devices", map[string]any{"deviceId": "device-1", "assigned": true}, adminCookie)
	if assigned.Code != http.StatusOK {
		t.Fatalf("assign device status = %d body=%s", assigned.Code, assigned.Body.String())
	}

	listed := requestJSON(t, handler, http.MethodGet, "/api/v1/users", nil, adminCookie)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(`"devices":["device-1"]`)) {
		t.Fatalf("assigned user response = %d %s", listed.Code, listed.Body.String())
	}

	userLogin := requestJSON(t, handler, http.MethodPost, "/api/v1/session", map[string]string{"username": "operator", "password": "operator-password"}, nil)
	if userLogin.Code != http.StatusForbidden {
		t.Fatalf("ordinary remote user Web login status = %d, want 403", userLogin.Code)
	}
}
