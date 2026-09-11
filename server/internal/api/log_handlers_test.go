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
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/observability"
)

func TestRequestAndAuditLogsArePersisted(t *testing.T) {
	// 场景：关键写操作必须同时形成 request 与 audit 日志，后续可按 traceId 排查。
	logs, manager := testLogService(t)
	server := &Server{config: config.Config{}, auth: manager, logs: logs, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler := server.Handler()
	login := authenticatedLogTestSession(t, handler)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}

	var body struct {
		Data []observability.Event `json:"data"`
	}
	query := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=audit", nil)
	request.AddCookie(login.Result().Cookies()[0])
	handler.ServeHTTP(query, request)
	if query.Code != http.StatusOK {
		t.Fatalf("query status = %d body=%s", query.Code, query.Body.String())
	}
	if err := json.Unmarshal(query.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) == 0 || body.Data[0].Action != "auth.login" || body.Data[0].TraceID == "" {
		t.Fatalf("missing audit log: %#v", body.Data)
	}
}

func TestClientErrorLogCanBeReportedAndQueried(t *testing.T) {
	// 场景：前端运行时异常必须能上报到服务端，后续可按错误码或全文检索定位。
	logs, manager := testLogService(t)
	server := &Server{config: config.Config{}, auth: manager, logs: logs, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler := server.Handler()
	login := authenticatedLogTestSession(t, handler)
	cookie := login.Result().Cookies()[0]

	report := httptest.NewRequest(http.MethodPost, "/api/v1/logs/client-error", bytes.NewBufferString(`{"message":"组件渲染失败","source":"vue","route":"/devices","errorCode":"WEB_RENDER_FAILED","details":{"token":"abc"}}`))
	report.Header.Set("Content-Type", "application/json")
	report.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, report)
	if response.Code != http.StatusCreated {
		t.Fatalf("report status = %d body=%s", response.Code, response.Body.String())
	}

	query := httptest.NewRequest(http.MethodGet, "/api/v1/logs?type=client_error&errorCode=WEB_RENDER_FAILED", nil)
	query.AddCookie(cookie)
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, query)
	var body struct {
		Data []observability.Event `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].Details["token"] != "[redacted]" {
		t.Fatalf("unexpected client error logs: %s", list.Body.String())
	}
}

func TestRemoteUserMutationsAreAudited(t *testing.T) {
	cases := map[string]string{
		"/api/v1/users":                   "remote_user.create",
		"/api/v1/users/operator":          "remote_user.delete",
		"/api/v1/users/operator/password": "remote_user.reset_password",
		"/api/v1/users/operator/devices":  "remote_user.assign_device",
	}
	for path, expected := range cases {
		method := http.MethodPut
		if path == "/api/v1/users" {
			method = http.MethodPost
		} else if path == "/api/v1/users/operator" {
			method = http.MethodDelete
		}
		request := httptest.NewRequest(method, path, nil)
		if !shouldAudit(request) || auditAction(request) != expected {
			t.Fatalf("audit route %s %s = %q", method, path, auditAction(request))
		}
	}
}

func authenticatedLogTestSession(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	const password = "test-admin-password"
	login := requestJSON(t, handler, http.MethodPost, "/api/v1/session", map[string]string{"username": auth.DefaultAdminUsername, "password": password}, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	return login
}

func testLogService(t *testing.T) (*observability.Service, *auth.Manager) {
	t.Helper()
	repository, err := observability.OpenRepository(filepath.Join(t.TempDir(), "observability.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	manager, core := newTestAuthManager()
	core.password = "test-admin-password"
	return observability.NewService(repository), manager
}
