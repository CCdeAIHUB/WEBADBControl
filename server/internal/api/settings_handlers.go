package api

import (
	"io"
	"net"
	"net/http"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/ai"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/settings"
)

func (s *Server) getSettings(writer http.ResponseWriter, _ *http.Request) {
	writeData(writer, http.StatusOK, s.settings.Get(false))
}

func (s *Server) saveSettings(writer http.ResponseWriter, request *http.Request) {
	var data settings.Settings
	if !decodeJSON(writer, request, &data) {
		return
	}
	if data.RemoteEnabled && net.ParseIP(data.RemoteAddress) == nil {
		writeError(writer, http.StatusBadRequest, apperror.New("REMOTE_LISTEN_ADDRESS_INVALID", "远程监听地址必须是有效 IP 地址", "api.settings", true))
		return
	}
	if data.RemoteEnabled && s.auth.DefaultPasswordActive(request.Context()) {
		writeError(writer, http.StatusConflict, apperror.New("REMOTE_DEFAULT_PASSWORD_ACTIVE", "开启远程网络前必须先修改默认管理员密码", "api.settings", true))
		return
	}
	if err := s.settings.Save(data); err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeData(writer, http.StatusOK, s.settings.Get(false))
}

func (s *Server) aiChat(writer http.ResponseWriter, request *http.Request) {
	var body ai.Request
	if !decodeJSON(writer, request, &body) {
		return
	}
	response, err := s.ai.Stream(request.Context(), body)
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	defer response.Body.Close()
	writer.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(response.StatusCode)
	if _, err := io.Copy(writer, response.Body); err != nil {
		s.logger.Warn("ai_stream_copy_failed", "traceId", writer.Header().Get("X-Request-ID"), "error", err)
	}
}
