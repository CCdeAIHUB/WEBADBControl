package api

import (
	"context"
	"net/http"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
)

func (s *Server) createQRPairing(writer http.ResponseWriter, request *http.Request) {
	result, err := s.devices.CreateWirelessPairingQR(request.Context())
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusCreated, result)
}

func (s *Server) pairQRDevice(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 105*time.Second)
	defer cancel()
	s.logger.Info("wireless_qr_pairing_wait_started", "traceId", writer.Header().Get("X-Request-ID"))
	result, err := s.devices.PairWirelessQR(ctx, request.PathValue("sessionId"))
	if err != nil {
		writeError(writer, deviceConnectionStatus(err, http.StatusBadGateway), err)
		return
	}
	s.logger.Info("wireless_qr_pairing_completed", "traceId", writer.Header().Get("X-Request-ID"), "serviceName", result.ServiceName, "endpoint", result.Endpoint)
	writeData(writer, http.StatusOK, result)
}

func (s *Server) cancelQRPairing(writer http.ResponseWriter, request *http.Request) {
	if err := s.devices.CancelWirelessQR(request.Context(), request.PathValue("sessionId")); err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, map[string]bool{"cancelled": true})
}

func (s *Server) discoverDevices(writer http.ResponseWriter, request *http.Request) {
	services, err := s.devices.Discover(request.Context())
	if err != nil {
		writeError(writer, deviceConnectionStatus(err, http.StatusBadGateway), err)
		return
	}
	writeData(writer, http.StatusOK, services)
}

func (s *Server) pairDevice(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
		Code     string `json:"code"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	if err := s.devices.Pair(request.Context(), body.Endpoint, body.Code); err != nil {
		writeError(writer, deviceConnectionStatus(err, http.StatusBadGateway), err)
		return
	}
	writeData(writer, http.StatusOK, map[string]any{"paired": true, "endpoint": body.Endpoint})
}

func (s *Server) disconnectDevice(writer http.ResponseWriter, request *http.Request) {
	if err := s.devices.Disconnect(request.Context(), request.PathValue("id")); err != nil {
		writeError(writer, deviceConnectionStatus(err, http.StatusBadGateway), err)
		return
	}
	writeData(writer, http.StatusOK, map[string]bool{"disconnected": true})
}

func (s *Server) removeDevice(writer http.ResponseWriter, request *http.Request) {
	session, ok := sessionFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, errUnauthorized())
		return
	}
	result, err := s.devices.Remove(request.Context(), request.PathValue("id"))
	if err != nil {
		s.logger.Warn("device_remove_failed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "disconnected", result.Disconnected, "error", err)
		writeError(writer, deviceConnectionStatus(err, http.StatusBadGateway), err)
		return
	}
	users, err := s.auth.ListRemoteUsers(request.Context(), session)
	if err != nil {
		s.logger.Warn("device_assignment_cleanup_failed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "error", err)
		writeAuthManagementError(writer, err)
		return
	}
	for _, account := range users {
		for _, assignedID := range account.Devices {
			if !matchesRemovedDevice(assignedID, result.Forgotten) {
				continue
			}
			if _, err := s.auth.SetRemoteUserDevice(request.Context(), session, account.Username, assignedID, false); err != nil {
				s.logger.Warn("device_assignment_cleanup_failed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "username", account.Username, "error", err)
				writeAuthManagementError(writer, err)
				return
			}
		}
	}
	s.logger.Info("device_removed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "disconnected", result.Disconnected)
	writeData(writer, http.StatusOK, result)
}

func matchesRemovedDevice(assignedID string, removedIDs []string) bool {
	for _, removedID := range removedIDs {
		if device.EquivalentDeviceID(assignedID, removedID) {
			return true
		}
	}
	return false
}

func (s *Server) enableDeviceTCPIP(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Port int `json:"port"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	if err := s.devices.EnableTCPIP(request.Context(), request.PathValue("id"), body.Port); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeData(writer, http.StatusOK, map[string]any{"enabled": true, "port": body.Port})
}

func (s *Server) keepAliveDevice(writer http.ResponseWriter, request *http.Request) {
	if err := s.devices.KeepAlive(request.Context(), request.PathValue("id")); err != nil {
		writeError(writer, deviceConnectionStatus(err, http.StatusBadGateway), err)
		return
	}
	writeData(writer, http.StatusOK, map[string]bool{"alive": true})
}

func (s *Server) deviceLockState(writer http.ResponseWriter, request *http.Request) {
	state, err := s.devices.LockState(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(writer, deviceConnectionStatus(err, http.StatusBadGateway), err)
		return
	}
	writeData(writer, http.StatusOK, state)
}

func (s *Server) unlockDevice(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		PIN string `json:"pin"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	if err := s.devices.Unlock(request.Context(), request.PathValue("id"), body.PIN); err != nil {
		writeError(writer, deviceConnectionStatus(err, http.StatusBadGateway), err)
		return
	}
	writeData(writer, http.StatusOK, map[string]bool{"unlocked": true})
}

func (s *Server) deviceHardware(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := s.devices.Hardware(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(writer, deviceConnectionStatus(err, http.StatusBadGateway), err)
		return
	}
	writeData(writer, http.StatusOK, snapshot)
}
