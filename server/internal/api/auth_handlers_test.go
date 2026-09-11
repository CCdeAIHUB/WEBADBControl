package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/auth"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/config"
)

func TestPasswordSessionLifecycle(t *testing.T) {
	manager, core := newTestAuthManager()
	server := &Server{config: config.Config{}, auth: manager, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler := server.Handler()

	protected := httptest.NewRecorder()
	remoteRequest := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	remoteRequest.RemoteAddr = "192.0.2.10:43000"
	handler.ServeHTTP(protected, remoteRequest)
	if protected.Code != http.StatusUnauthorized {
		t.Fatalf("protected endpoint status = %d, want 401", protected.Code)
	}

	login := requestJSON(t, handler, http.MethodPost, "/api/v1/session", map[string]string{"username": auth.DefaultAdminUsername, "password": auth.DefaultPassword}, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("default login status = %d body=%s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	var loginBody struct {
		Data sessionResponse `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &loginBody); err != nil {
		t.Fatal(err)
	}
	if !loginBody.Data.Authenticated || !loginBody.Data.MustChangePassword {
		t.Fatalf("unexpected login response: %+v", loginBody.Data)
	}
	changed := requestJSON(t, handler, http.MethodPut, "/api/v1/password", map[string]string{
		"currentPassword": auth.DefaultPassword,
		"newPassword":     "new-password-123",
	}, cookie)
	if changed.Code != http.StatusOK {
		t.Fatalf("change password status = %d body=%s", changed.Code, changed.Body.String())
	}
	if core.password != "new-password-123" {
		t.Fatal("Web password change did not update the Core administrator password")
	}
	newCookie := changed.Result().Cookies()[0]

	oldSession := httptest.NewRecorder()
	oldRequest := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	oldRequest.AddCookie(cookie)
	handler.ServeHTTP(oldSession, oldRequest)
	if oldSession.Code != http.StatusUnauthorized {
		t.Fatalf("old session status = %d, want 401", oldSession.Code)
	}

	newSession := httptest.NewRecorder()
	newRequest := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	newRequest.AddCookie(newCookie)
	handler.ServeHTTP(newSession, newRequest)
	if newSession.Code != http.StatusOK || !bytes.Contains(newSession.Body.Bytes(), []byte(`"mustChangePassword":false`)) {
		t.Fatalf("new session response = %d %s", newSession.Code, newSession.Body.String())
	}
}

func TestLoopbackBypassesLoginOnlyForCoreDefaultPassword(t *testing.T) {
	manager, core := newTestAuthManager()
	server := &Server{config: config.Config{}, auth: manager, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler := server.Handler()
	local := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	local.RemoteAddr = "127.0.0.1:41000"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, local)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"localBypass":true`)) {
		t.Fatalf("loopback session response: %d %s", response.Code, response.Body.String())
	}
	core.mu.Lock()
	core.password = "changed-password"
	core.mu.Unlock()
	response = httptest.NewRecorder()
	local = httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	local.RemoteAddr = "127.0.0.1:41000"
	handler.ServeHTTP(response, local)
	if !bytes.Contains(response.Body.Bytes(), []byte(`"authenticated":false`)) {
		t.Fatalf("loopback bypass must stop after password change: %d %s", response.Code, response.Body.String())
	}
}

func TestLoopbackOnlyListenerTreatsAppProxyAsLocal(t *testing.T) {
	manager, _ := newTestAuthManager()
	server := &Server{config: config.Config{Address: "127.0.0.1:8080"}, auth: manager, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	request.RemoteAddr = "192.0.2.10:43000"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if !bytes.Contains(response.Body.Bytes(), []byte(`"localBypass":true`)) {
		t.Fatalf("loopback-only listener should allow local app proxy: %d %s", response.Code, response.Body.String())
	}
}

func TestRemoteUserCannotLoginToWebConsole(t *testing.T) {
	manager, _ := newTestAuthManager()
	server := &Server{config: config.Config{}, auth: manager, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	response := requestJSON(t, server.Handler(), http.MethodPost, "/api/v1/session", map[string]string{
		"username": "operator",
		"password": "operator-password",
	}, nil)
	if response.Code != http.StatusForbidden || !bytes.Contains(response.Body.Bytes(), []byte(`"errorCode":"AUTH_ADMIN_ONLY"`)) {
		t.Fatalf("remote user login response = %d %s", response.Code, response.Body.String())
	}
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
