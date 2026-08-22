package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/auth"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/config"
)

func TestPasswordSessionLifecycle(t *testing.T) {
	manager, err := auth.Open(filepath.Join(t.TempDir(), "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{config: config.Config{}, auth: manager, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler := server.Handler()

	protected := httptest.NewRecorder()
	handler.ServeHTTP(protected, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if protected.Code != http.StatusUnauthorized {
		t.Fatalf("protected endpoint status = %d, want 401", protected.Code)
	}

	login := requestJSON(t, handler, http.MethodPost, "/api/v1/session", map[string]string{"password": auth.DefaultPassword}, nil)
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

	blocked := httptest.NewRecorder()
	blockedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	blockedRequest.AddCookie(cookie)
	handler.ServeHTTP(blocked, blockedRequest)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("bootstrap session device status = %d, want 403", blocked.Code)
	}

	changed := requestJSON(t, handler, http.MethodPut, "/api/v1/password", map[string]string{
		"currentPassword": auth.DefaultPassword,
		"newPassword":     "new-password-123",
	}, cookie)
	if changed.Code != http.StatusOK {
		t.Fatalf("change password status = %d body=%s", changed.Code, changed.Body.String())
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
