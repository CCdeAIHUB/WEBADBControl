package api

import (
	"crypto/subtle"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/ai"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/auth"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/automation"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/config"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/settings"
)

type Server struct {
	config     config.Config
	devices    *device.Service
	automation *automation.Service
	settings   *settings.Store
	ai         *ai.Service
	auth       *auth.Manager
	logger     *slog.Logger
}

func New(config config.Config, devices *device.Service, automation *automation.Service, settings *settings.Store, ai *ai.Service, auth *auth.Manager, logger *slog.Logger) *Server {
	return &Server{config: config, devices: devices, automation: automation, settings: settings, ai: ai, auth: auth, logger: logger}
}

func (s *Server) Handler() http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("GET /api/v1/health", s.health)
	router.HandleFunc("GET /api/v1/session", s.sessionStatus)
	router.HandleFunc("POST /api/v1/session", s.createSession)
	router.HandleFunc("DELETE /api/v1/session", s.deleteSession)
	router.HandleFunc("PUT /api/v1/password", s.changePassword)
	router.HandleFunc("GET /api/v1/overview", s.overview)
	router.HandleFunc("GET /api/v1/devices", s.listDevices)
	router.HandleFunc("GET /api/v1/devices/discover", s.discoverDevices)
	router.HandleFunc("POST /api/v1/devices/connect", s.connectDevice)
	router.HandleFunc("POST /api/v1/devices/pair", s.pairDevice)
	router.HandleFunc("GET /api/v1/devices/{id}/overview", s.deviceOverview)
	router.HandleFunc("GET /api/v1/devices/{id}/hardware", s.deviceHardware)
	router.HandleFunc("GET /api/v1/devices/{id}/lock", s.deviceLockState)
	router.HandleFunc("POST /api/v1/devices/{id}/unlock", s.unlockDevice)
	router.HandleFunc("POST /api/v1/devices/{id}/disconnect", s.disconnectDevice)
	router.HandleFunc("POST /api/v1/devices/{id}/tcpip", s.enableDeviceTCPIP)
	router.HandleFunc("POST /api/v1/devices/{id}/actions", s.deviceAction)
	router.HandleFunc("GET /api/v1/devices/{id}/screenshot", s.screenshot)
	router.HandleFunc("GET /api/v1/devices/{id}/screen", s.screenSocket)
	router.HandleFunc("POST /api/v1/devices/{id}/terminal", s.terminal)
	router.HandleFunc("GET /api/v1/devices/{id}/packages", s.packages)
	router.HandleFunc("POST /api/v1/devices/{id}/packages/action", s.packageAction)
	router.HandleFunc("POST /api/v1/devices/{id}/packages/install", s.installPackage)
	router.HandleFunc("GET /api/v1/devices/{id}/files", s.files)
	router.HandleFunc("POST /api/v1/devices/{id}/files/upload", s.uploadFile)
	router.HandleFunc("GET /api/v1/devices/{id}/files/download", s.downloadFile)
	router.HandleFunc("GET /api/v1/devices/{id}/capabilities", s.capabilities)
	router.HandleFunc("GET /api/v1/devices/{id}/permissions", s.permissions)
	router.HandleFunc("POST /api/v1/devices/{id}/capabilities/invoke", s.invokeCapability)
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
		sessionAuthorized := false
		if cookie, err := request.Cookie(sessionCookieName); err == nil {
			sessionAuthorized = s.auth.ValidateSession(cookie.Value)
		}
		if !legacyAuthorized && !sessionAuthorized {
			writeError(writer, http.StatusUnauthorized, errUnauthorized())
			return
		}
		if sessionAuthorized && s.auth.MustChangePassword() && request.URL.Path != "/api/v1/password" && request.URL.Path != "/api/v1/settings" {
			writeError(writer, http.StatusForbidden, errPasswordChangeRequired())
			return
		}
		next.ServeHTTP(writer, request)
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
		next.ServeHTTP(writer, request)
		s.logger.Info("http_request", "method", request.Method, "path", request.URL.Path, "durationMs", time.Since(started).Milliseconds())
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
	_, _ = io.Copy(writer, file)
}
