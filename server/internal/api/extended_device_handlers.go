package api

import (
	"net/http"
)

func (s *Server) discoverDevices(writer http.ResponseWriter, request *http.Request) {
	services, err := s.devices.Discover(request.Context())
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
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
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, map[string]any{"paired": true, "endpoint": body.Endpoint})
}

func (s *Server) disconnectDevice(writer http.ResponseWriter, request *http.Request) {
	if err := s.devices.Disconnect(request.Context(), request.PathValue("id")); err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, map[string]bool{"disconnected": true})
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

func (s *Server) deviceLockState(writer http.ResponseWriter, request *http.Request) {
	state, err := s.devices.LockState(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
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
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, map[string]bool{"unlocked": true})
}

func (s *Server) deviceHardware(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := s.devices.Hardware(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, snapshot)
}
