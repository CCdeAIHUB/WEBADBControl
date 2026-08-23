package api

import (
	"net/http"
	"strconv"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/observability"
)

func (s *Server) listLogs(writer http.ResponseWriter, request *http.Request) {
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	events, err := s.logs.List(request.Context(), observability.Query{
		Type:      request.URL.Query().Get("type"),
		Level:     request.URL.Query().Get("level"),
		Module:    request.URL.Query().Get("module"),
		Action:    request.URL.Query().Get("action"),
		TraceID:   request.URL.Query().Get("traceId"),
		ErrorCode: request.URL.Query().Get("errorCode"),
		DeviceID:  request.URL.Query().Get("deviceId"),
		Text:      request.URL.Query().Get("q"),
		Limit:     limit,
	})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, apperror.Wrap("LOG_QUERY_FAILED", "日志查询失败", "observability.query", true, err))
		return
	}
	writeData(writer, http.StatusOK, events)
}

func (s *Server) logStats(writer http.ResponseWriter, request *http.Request) {
	stats, err := s.logs.Stats(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, apperror.Wrap("LOG_STATS_FAILED", "日志统计失败", "observability.query", true, err))
		return
	}
	writeData(writer, http.StatusOK, stats)
}

func (s *Server) clientError(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Message   string         `json:"message"`
		Source    string         `json:"source"`
		Stack     string         `json:"stack"`
		Component string         `json:"component"`
		Route     string         `json:"route"`
		ErrorCode string         `json:"errorCode"`
		TraceID   string         `json:"traceId"`
		Details   map[string]any `json:"details"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	if body.Message == "" {
		writeError(writer, http.StatusBadRequest, apperror.New("CLIENT_LOG_INVALID", "客户端错误日志缺少 message", "observability.client", true))
		return
	}
	traceID := request.Header.Get("X-Request-ID")
	if body.TraceID != "" {
		traceID = body.TraceID
	}
	details := map[string]any{
		"source":    body.Source,
		"component": body.Component,
		"route":     body.Route,
		"stack":     body.Stack,
	}
	for key, value := range body.Details {
		details[key] = value
	}
	if err := s.logs.Record(request.Context(), observability.Event{
		Type:      observability.TypeClientError,
		Level:     observability.LevelError,
		Module:    "web.client",
		Action:    "client.error",
		Message:   body.Message,
		TraceID:   traceID,
		ErrorCode: body.ErrorCode,
		Path:      body.Route,
		Actor:     actorForRequest(request),
		IPAddress: clientIP(request),
		UserAgent: request.UserAgent(),
		Details:   details,
	}); err != nil {
		writeError(writer, http.StatusInternalServerError, apperror.Wrap("CLIENT_LOG_SAVE_FAILED", "客户端错误日志保存失败", "observability.client", true, err))
		return
	}
	writeData(writer, http.StatusCreated, map[string]bool{"accepted": true})
}
