package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

func writeData(writer http.ResponseWriter, status int, data any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"data": data})
}

func writeError(writer http.ResponseWriter, status int, err error) {
	var appError *apperror.Error
	if !errors.As(err, &appError) {
		appError = apperror.Wrap("INTERNAL_ERROR", "服务暂时不可用", "api", true, err)
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": appError})
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, 2*1024*1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(writer, http.StatusBadRequest, apperror.Wrap("REQUEST_INVALID", "请求内容无效", "api.request", true, err))
		return false
	}
	return true
}
