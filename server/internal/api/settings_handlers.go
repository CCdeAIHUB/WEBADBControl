package api

import (
	"io"
	"net/http"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/ai"
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
	_, _ = io.Copy(writer, response.Body)
}
