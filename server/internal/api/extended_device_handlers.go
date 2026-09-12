package api

import (
	"net/http"
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
	result, err := s.devices.PairWirelessQR(request.Context(), request.PathValue("sessionId"))
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
	result, err := s.devices.Remove(request.Context(), request.PathValue("id"))
	if err != nil {
		s.logger.Warn("device_remove_failed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "disconnected", result.Disconnected, "error", err)
		writeError(writer, deviceConnectionStatus(err, http.StatusBadGateway), err)
		return
	}
	s.logger.Info("device_removed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "disconnected", result.Disconnected)
	writeData(writer, http.StatusOK, result)
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
