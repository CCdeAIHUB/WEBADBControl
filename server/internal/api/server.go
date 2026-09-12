package api

import (
	"crypto/subtle"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/ai"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/auth"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/automation"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/config"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/observability"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/settings"
)

type Server struct {
	config     config.Config
	devices    *device.Service
	automation *automation.Service
	settings   *settings.Store
	ai         *ai.Service
	auth       *auth.Manager
	logs       *observability.Service
	logger     *slog.Logger
}

func New(config config.Config, devices *device.Service, automation *automation.Service, settings *settings.Store, ai *ai.Service, auth *auth.Manager, logs *observability.Service, logger *slog.Logger) *Server {
	return &Server{config: config, devices: devices, automation: automation, settings: settings, ai: ai, auth: auth, logs: logs, logger: logger}
}

func (s *Server) Handler() http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("GET /api/v1/health", s.health)
	router.HandleFunc("GET /api/v1/session", s.sessionStatus)
	router.HandleFunc("POST /api/v1/session", s.createSession)
	router.HandleFunc("DELETE /api/v1/session", s.deleteSession)
	router.HandleFunc("PUT /api/v1/password", s.changePassword)
	router.HandleFunc("GET /api/v1/users", s.listUsers)
	router.HandleFunc("POST /api/v1/users", s.createUser)
	router.HandleFunc("DELETE /api/v1/users/{username}", s.deleteUser)
	router.HandleFunc("PUT /api/v1/users/{username}/password", s.resetUserPassword)
	router.HandleFunc("PUT /api/v1/users/{username}/devices", s.assignUserDevice)
	router.HandleFunc("GET /api/v1/overview", s.overview)
	router.HandleFunc("GET /api/v1/devices", s.listDevices)
	router.HandleFunc("GET /api/v1/devices/discover", s.discoverDevices)
	router.HandleFunc("POST /api/v1/devices/connect", s.connectDevice)
	router.HandleFunc("POST /api/v1/devices/pair", s.pairDevice)
	router.HandleFunc("POST /api/v1/wireless/qr-pairings", s.createQRPairing)
	router.HandleFunc("POST /api/v1/wireless/qr-pairings/{sessionId}/pair", s.pairQRDevice)
	router.HandleFunc("DELETE /api/v1/wireless/qr-pairings/{sessionId}", s.cancelQRPairing)
	router.HandleFunc("GET /api/v1/devices/{id}/overview", s.deviceOverview)
	router.HandleFunc("GET /api/v1/devices/{id}/hardware", s.deviceHardware)
	router.HandleFunc("GET /api/v1/devices/{id}/lock", s.deviceLockState)
	router.HandleFunc("POST /api/v1/devices/{id}/unlock", s.unlockDevice)
	router.HandleFunc("POST /api/v1/devices/{id}/disconnect", s.disconnectDevice)
	router.HandleFunc("POST /api/v1/devices/{id}/remove", s.removeDevice)
	router.HandleFunc("POST /api/v1/devices/{id}/tcpip", s.enableDeviceTCPIP)
	router.HandleFunc("POST /api/v1/devices/{id}/keepalive", s.keepAliveDevice)
	router.HandleFunc("POST /api/v1/devices/{id}/actions", s.deviceAction)
	router.HandleFunc("GET /api/v1/devices/{id}/screenshot", s.screenshot)
	router.HandleFunc("GET /api/v1/devices/{id}/screen", s.screenSocket)
	router.HandleFunc("POST /api/v1/devices/{id}/terminal", s.terminal)
	router.HandleFunc("GET /api/v1/devices/{id}/packages", s.packages)
	router.HandleFunc("POST /api/v1/devices/{id}/packages/action", s.packageAction)
	router.HandleFunc("POST /api/v1/devices/{id}/packages/install", s.installPackage)
	router.HandleFunc("GET /api/v1/devices/{id}/files", s.files)
	router.HandleFunc("DELETE /api/v1/devices/{id}/files", s.deleteFile)
	router.HandleFunc("POST /api/v1/devices/{id}/files/mkdir", s.createDirectory)
	router.HandleFunc("POST /api/v1/devices/{id}/files/upload", s.uploadFile)
	router.HandleFunc("GET /api/v1/devices/{id}/files/download", s.downloadFile)
	router.HandleFunc("GET /api/v1/devices/{id}/capabilities", s.capabilities)
	router.HandleFunc("GET /api/v1/devices/{id}/permissions", s.permissions)
	router.HandleFunc("POST /api/v1/devices/{id}/capabilities/invoke", s.invokeCapability)
	router.HandleFunc("GET /api/v1/devices/{id}/companion/status", s.companionStatus)
	router.HandleFunc("POST /api/v1/devices/{id}/companion/install", s.installCompanion)
	router.HandleFunc("GET /api/v1/automation/tasks", s.listTasks)
	router.HandleFunc("POST /api/v1/automation/tasks", s.saveTask)
	router.HandleFunc("PUT /api/v1/automation/tasks/{id}", s.saveTask)
	router.HandleFunc("DELETE /api/v1/automation/tasks/{id}", s.deleteTask)
	router.HandleFunc("POST /api/v1/automation/tasks/{id}/run", s.runTask)
	router.HandleFunc("GET /api/v1/automation/runs", s.listRuns)
	router.HandleFunc("POST /api/v1/automation/runs/{id}/{operation}", s.controlRun)
	router.HandleFunc("GET /api/v1/settings", s.getSettings)
	router.HandleFunc("PUT /api/v1/settings", s.saveSettings)
	router.HandleFunc("POST /api/v1/ai/chat", s.aiChat)
	router.HandleFunc("GET /api/v1/logs", s.listLogs)
	router.HandleFunc("GET /api/v1/logs/stats", s.logStats)
	router.HandleFunc("POST /api/v1/logs/client-error", s.clientError)
	router.HandleFunc("/", s.static)
	return s.logging(s.securityHeaders(s.authentication(router)))
}

func (s *Server) health(writer http.ResponseWriter, _ *http.Request) {
	writeData(writer, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
}

func (s *Server) authentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/health" || request.URL.Path == "/api/v1/session" || !strings.HasPrefix(request.URL.Path, "/api/") {
			next.ServeHTTP(writer, request)
			return
		}
		token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		legacyAuthorized := token != "" && s.validLegacyToken(token)
		var session auth.Session
		sessionAuthorized := false
		if cookie, err := request.Cookie(sessionCookieName); err == nil {
			session, sessionAuthorized = s.auth.Authenticate(cookie.Value)
		}
		if !sessionAuthorized && isLocalManagementRequest(s.config.Address, request) {
			session, sessionAuthorized = s.auth.LocalBootstrapSession(request.Context())
		}
		if legacyAuthorized {
			session = auth.Session{Username: "legacy-token", Role: auth.RoleAdmin}
			sessionAuthorized = true
		}
		if !sessionAuthorized {
			writeError(writer, http.StatusUnauthorized, errUnauthorized())
			return
		}
		if session.PasswordChangeRequired && !session.LocalBypass && request.URL.Path != "/api/v1/password" && request.URL.Path != "/api/v1/session" {
			writeError(writer, http.StatusForbidden, errPasswordChangeRequired())
			return
		}
		if session.Role != auth.RoleAdmin {
			writeError(writer, http.StatusForbidden, errForbidden())
			return
		}
		next.ServeHTTP(writer, withSession(request, session))
	})
}

func (s *Server) validLegacyToken(token string) bool {
	if s.config.AuthToken == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.config.AuthToken)) == 1
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "same-origin")
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self' ws: wss:; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'")
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		traceID := requestTraceID()
		writer.Header().Set("X-Request-ID", traceID)
		state := &responseStateWriter{ResponseWriter: writer}
		next.ServeHTTP(state, request)
		status := state.status
		if status == 0 {
			status = http.StatusOK
		}
		if state.writeErr != nil {
			s.logger.Warn("http_response_write_failed", "method", request.Method, "path", request.URL.Path, "status", status, "traceId", state.Header().Get("X-Request-ID"), "error", state.writeErr)
		}
		duration := time.Since(started).Milliseconds()
		traceID = state.Header().Get("X-Request-ID")
		errorCode := state.Header().Get("X-App-Error-Code")
		if !isUnauthenticatedClientLog(request.URL.Path, status) {
			s.logger.Info("http_request", "method", request.Method, "path", request.URL.Path, "status", status, "traceId", traceID, "errorCode", errorCode, "durationMs", duration)
		}
		s.recordRequestLog(request, status, duration, traceID, errorCode)
	})
}

func (s *Server) static(writer http.ResponseWriter, request *http.Request) {
	relativePath := strings.TrimPrefix(filepath.Clean(request.URL.Path), string(filepath.Separator))
	path := filepath.Join(s.config.WebDir, relativePath)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		if contentType := mime.TypeByExtension(filepath.Ext(path)); contentType != "" {
			writer.Header().Set("Content-Type", contentType)
		}
		http.ServeFile(writer, request, path)
		return
	}
	index := filepath.Join(s.config.WebDir, "index.html")
	file, err := os.Open(index)
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	defer file.Close()
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := io.Copy(writer, file); err != nil {
		s.logger.Warn("static_response_copy_failed", "traceId", writer.Header().Get("X-Request-ID"), "path", request.URL.Path, "error", err)
	}
}

var devicePathPattern = regexp.MustCompile(`/api/v1/devices/([^/]+)`)

func (s *Server) recordRequestLog(request *http.Request, status int, duration int64, traceID, errorCode string) {
	if s.logs == nil || !strings.HasPrefix(request.URL.Path, "/api/") || request.URL.Path == "/api/v1/health" {
		return
	}
	if isUnauthenticatedClientLog(request.URL.Path, status) {
		return
	}
	level := observability.LevelInfo
	if status >= 500 {
		level = observability.LevelError
	} else if status >= 400 {
		level = observability.LevelWarn
	}
	action := request.Method + " " + request.URL.Path
	deviceID := ""
	if matches := devicePathPattern.FindStringSubmatch(request.URL.Path); len(matches) == 2 {
		deviceID = matches[1]
	}
	event := observability.Event{
		Type:       observability.TypeRequest,
		Level:      level,
		Module:     moduleForPath(request.URL.Path),
		Action:     action,
		Message:    "HTTP " + action,
		TraceID:    traceID,
		ErrorCode:  errorCode,
		DeviceID:   deviceID,
		Method:     request.Method,
		Path:       request.URL.Path,
		Status:     status,
		DurationMS: duration,
		Actor:      actorForRequest(request),
		IPAddress:  clientIP(request),
		UserAgent:  request.UserAgent(),
		Details: map[string]any{
			"query": sanitizedQueryKeys(request.URL.Query()),
		},
	}
	if err := s.logs.Record(request.Context(), event); err != nil {
		s.logger.Warn("observability_request_log_failed", "traceId", traceID, "error", err)
	}
	if shouldAudit(request) {
		event.Type = observability.TypeAudit
		event.Action = auditAction(request)
		event.Message = auditMessage(request, status)
		if err := s.logs.Record(request.Context(), event); err != nil {
			s.logger.Warn("observability_audit_log_failed", "traceId", traceID, "error", err)
		}
	}
}

func isUnauthenticatedClientLog(path string, status int) bool {
	return path == "/api/v1/logs/client-error" && status == http.StatusUnauthorized
}

func moduleForPath(path string) string {
	switch {
	case strings.Contains(path, "/session") || strings.Contains(path, "/password") || strings.Contains(path, "/users"):
		return "api.auth"
	case strings.Contains(path, "/devices"):
		return "device"
	case strings.Contains(path, "/automation"):
		return "automation"
	case strings.Contains(path, "/settings"):
		return "settings"
	case strings.Contains(path, "/ai/"):
		return "ai"
	case strings.Contains(path, "/logs"):
		return "observability"
	default:
		return "api"
	}
}

func shouldAudit(request *http.Request) bool {
	if request.Method == http.MethodGet {
		return false
	}
	path := request.URL.Path
	if path == "/api/v1/logs/client-error" {
		return false
	}
	return strings.Contains(path, "/session") ||
		strings.Contains(path, "/password") ||
		strings.Contains(path, "/users") ||
		strings.Contains(path, "/devices") ||
		strings.Contains(path, "/automation") ||
		strings.Contains(path, "/settings") ||
		strings.Contains(path, "/ai/")
}

func auditAction(request *http.Request) string {
	path := request.URL.Path
	switch {
	case path == "/api/v1/session" && request.Method == http.MethodPost:
		return "auth.login"
	case path == "/api/v1/session" && request.Method == http.MethodDelete:
		return "auth.logout"
	case path == "/api/v1/password":
		return "auth.change_password"
	case path == "/api/v1/users" && request.Method == http.MethodPost:
		return "remote_user.create"
	case strings.Contains(path, "/users/") && request.Method == http.MethodDelete:
		return "remote_user.delete"
	case strings.HasSuffix(path, "/password") && strings.Contains(path, "/users/"):
		return "remote_user.reset_password"
	case strings.HasSuffix(path, "/devices") && strings.Contains(path, "/users/"):
		return "remote_user.assign_device"
	case strings.Contains(path, "/devices/pair"):
		return "device.pair"
	case strings.Contains(path, "/devices/connect"):
		return "device.connect"
	case strings.Contains(path, "/actions"):
		return "device.action"
	case strings.Contains(path, "/terminal"):
		return "device.terminal"
	case strings.Contains(path, "/files"):
		return "device.files"
	case strings.Contains(path, "/packages"):
		return "device.packages"
	case strings.Contains(path, "/automation"):
		return "automation.change"
	case strings.Contains(path, "/settings"):
		return "settings.save"
	case strings.Contains(path, "/ai/"):
		return "ai.chat"
	default:
		return request.Method + " " + path
	}
}

func auditMessage(request *http.Request, status int) string {
	result := "成功"
	if status >= 400 {
		result = "失败"
	}
	return auditAction(request) + " " + result
}

func actorForRequest(request *http.Request) string {
	if _, err := request.Cookie(sessionCookieName); err == nil {
		return "session"
	}
	if strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ") != "" {
		return "legacy-token"
	}
	return "anonymous"
}

func clientIP(request *http.Request) string {
	if forwarded := strings.TrimSpace(request.Header.Get("X-Forwarded-For")); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host := request.RemoteAddr
	if index := strings.LastIndex(host, ":"); index > 0 {
		return host[:index]
	}
	return host
}

func sanitizedQueryKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "key") || strings.Contains(lower, "secret") {
			keys = append(keys, key+"=[redacted]")
			continue
		}
		keys = append(keys, key)
	}
	return keys
}
