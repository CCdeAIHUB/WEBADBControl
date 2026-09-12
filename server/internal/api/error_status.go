package api

import (
	"errors"
	"net/http"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

func deviceConnectionStatus(err error, fallback int) int {
	var appError *apperror.Error
	if !errors.As(err, &appError) {
		return fallback
	}
	switch appError.ErrorCode {
	case "ADB_ENDPOINT_INVALID", "ADB_PAIR_CODE_INVALID", "ADB_QR_PAIRING_SESSION_INVALID":
		return http.StatusBadRequest
	case "ADB_PAIR_UNSUPPORTED", "ADB_MDNS_UNSUPPORTED":
		return http.StatusServiceUnavailable
	default:
		return fallback
	}
}
