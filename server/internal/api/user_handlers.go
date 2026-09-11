package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/auth"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
)

func (s *Server) listUsers(writer http.ResponseWriter, request *http.Request) {
	session, ok := sessionFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, errUnauthorized())
		return
	}
	users, err := s.auth.ListRemoteUsers(request.Context(), session)
	if err != nil {
		writeAuthManagementError(writer, err)
		return
	}
	writeData(writer, http.StatusOK, users)
}

func (s *Server) createUser(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	session, ok := sessionFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, errUnauthorized())
		return
	}
	account, err := s.auth.CreateRemoteUser(request.Context(), session, body.Username, body.Password)
	if err != nil {
		writeAuthManagementError(writer, err)
		return
	}
	writeData(writer, http.StatusCreated, account)
}

func (s *Server) deleteUser(writer http.ResponseWriter, request *http.Request) {
	session, ok := sessionFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, errUnauthorized())
		return
	}
	if err := s.auth.DeleteRemoteUser(request.Context(), session, request.PathValue("username")); err != nil {
		writeAuthManagementError(writer, err)
		return
	}
	writeData(writer, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) resetUserPassword(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		NewPassword string `json:"newPassword"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	session, ok := sessionFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, errUnauthorized())
		return
	}
	if err := s.auth.ResetRemoteUserPassword(
		request.Context(),
		session,
		request.PathValue("username"),
		body.NewPassword,
	); err != nil {
		writeAuthManagementError(writer, err)
		return
	}
	writeData(writer, http.StatusOK, map[string]bool{"passwordReset": true, "loginRequired": true})
}

func (s *Server) assignUserDevice(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		DeviceID string `json:"deviceId"`
		Assigned *bool  `json:"assigned"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	body.DeviceID = strings.TrimSpace(body.DeviceID)
	if body.DeviceID == "" {
		writeError(writer, http.StatusBadRequest, apperror.New("REMOTE_AUTH_DEVICE_ID_INVALID", "设备标识不能为空", "api.auth", true))
		return
	}
	assigned := body.Assigned == nil || *body.Assigned
	if assigned {
		devices, err := s.devices.List(request.Context())
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, err)
			return
		}
		if !containsDevice(devices, body.DeviceID) {
			writeError(writer, http.StatusConflict, apperror.New("REMOTE_AUTH_DEVICE_NOT_CONNECTED", "只能分配当前已连接的设备", "api.auth", true))
			return
		}
	}
	session, ok := sessionFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, errUnauthorized())
		return
	}
	account, err := s.auth.SetRemoteUserDevice(
		request.Context(),
		session,
		request.PathValue("username"),
		body.DeviceID,
		assigned,
	)
	if err != nil {
		writeAuthManagementError(writer, err)
		return
	}
	writeData(writer, http.StatusOK, account)
}

func containsDevice(devices []device.Device, deviceID string) bool {
	for _, candidate := range devices {
		if candidate.ID == deviceID {
			return true
		}
		for _, alias := range candidate.Aliases {
			if alias == deviceID {
				return true
			}
		}
	}
	return false
}

func writeAuthManagementError(writer http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrAdminOnly) {
		writeError(writer, http.StatusForbidden, apperror.New("AUTH_ADMIN_ONLY", "仅内置管理员会话可以管理远程用户", "api.auth", false))
		return
	}
	status := http.StatusBadRequest
	switch appErrorCode(err) {
	case "REMOTE_AUTH_USER_NOT_FOUND":
		status = http.StatusNotFound
	case "REMOTE_AUTH_USER_EXISTS":
		status = http.StatusConflict
	case "REMOTE_AUTH_FORBIDDEN", "REMOTE_AUTH_BUILTIN_USER_IMMUTABLE":
		status = http.StatusForbidden
	case "REMOTE_AUTH_SESSION_INVALID":
		status = http.StatusUnauthorized
	case "REMOTE_AUTH_STATE_UNAVAILABLE", "REMOTE_AUTH_READ_FAILED", "REMOTE_AUTH_WRITE_FAILED":
		status = http.StatusServiceUnavailable
	}
	writeError(writer, status, err)
}
