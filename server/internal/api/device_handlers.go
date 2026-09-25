package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
	"github.com/gorilla/websocket"
)

var packagePattern = regexp.MustCompile(`^[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)+$`)

func (s *Server) overview(writer http.ResponseWriter, request *http.Request) {
	devices, err := s.devices.List(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	tasks, err := s.automation.List(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	runs, err := s.automation.Runs(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeData(writer, http.StatusOK, map[string]any{"devices": devices, "taskCount": len(tasks), "recentRuns": runs})
}

func (s *Server) listDevices(writer http.ResponseWriter, request *http.Request) {
	devices, err := s.devices.List(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	writeData(writer, http.StatusOK, devices)
}

func (s *Server) connectDevice(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	if err := s.devices.Connect(request.Context(), body.Endpoint); err != nil {
		writeError(writer, deviceConnectionStatus(err, http.StatusBadGateway), err)
		return
	}
	writeData(writer, http.StatusOK, map[string]string{"endpoint": body.Endpoint, "state": "online"})
}

func (s *Server) deviceOverview(writer http.ResponseWriter, request *http.Request) {
	data, err := s.devices.Overview(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, data)
}

func (s *Server) deviceAction(writer http.ResponseWriter, request *http.Request) {
	var body device.ActionRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	ctx, cancel := withTimeout(request, 5*time.Second)
	defer cancel()
	outcome, err := s.devices.ActionWithOutcome(ctx, request.PathValue("id"), body)
	if err != nil {
		s.logger.Warn("device_action_failed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "actionType", body.Type, "key", body.Key, "transport", outcome.Transport, "error", err)
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	s.logger.Info("device_action_completed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "actionType", body.Type, "key", body.Key, "transport", outcome.Transport)
	writeData(writer, http.StatusOK, map[string]any{"accepted": true, "transport": outcome.Transport})
}

func (s *Server) screenshot(writer http.ResponseWriter, request *http.Request) {
	png, err := s.devices.Screenshot(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writer.Header().Set("Content-Type", "image/png")
	writer.Header().Set("Cache-Control", "no-store")
	if _, err := writer.Write(png); err != nil {
		s.logger.Warn("screenshot_response_write_failed", "traceId", writer.Header().Get("X-Request-ID"), "error", err)
	}
}

var screenUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024 * 1024,
	CheckOrigin: func(request *http.Request) bool {
		origin := request.Header.Get("Origin")
		return origin == "" || strings.Contains(origin, request.Host)
	},
}

func (s *Server) screenSocket(writer http.ResponseWriter, request *http.Request) {
	connection, err := screenUpgrader.Upgrade(writer, request, nil)
	if err != nil {
		s.logger.Warn("screen_websocket_upgrade_failed", "traceId", writer.Header().Get("X-Request-ID"), "error", err)
		return
	}
	defer connection.Close()
	frameRate := screenFrameRate(request.URL.Query().Get("fps"), 30)
	session, err := s.devices.StartScrcpy(request.Context(), request.PathValue("id"), device.ScrcpyOptions{FrameRate: frameRate})
	if err != nil {
		s.logger.Error("screen_start_failed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "fps", frameRate, "error", err)
		_ = connection.WriteJSON(map[string]any{"type": "error", "message": "无法启动 H.264 投屏：" + err.Error()})
		return
	}
	defer session.Close()
	width, height := session.Size()
	s.logger.Info("screen_websocket_started", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "backend", "scrcpy-h264", "width", width, "height", height, "fps", frameRate)
	if err := connection.WriteJSON(map[string]any{"type": "meta", "codec": "h264", "width": width, "height": height}); err != nil {
		return
	}
	go func() {
		for {
			var control struct {
				Type      string `json:"type"`
				Action    int    `json:"action"`
				PointerID uint32 `json:"pointerId"`
				X         int    `json:"x"`
				Y         int    `json:"y"`
				Width     int    `json:"width"`
				Height    int    `json:"height"`
			}
			if err := connection.ReadJSON(&control); err != nil {
				session.Close()
				return
			}
			if control.Type != "touch" {
				continue
			}
			if err := session.SendTouch(control.Action, control.PointerID, control.X, control.Y, control.Width, control.Height); err != nil {
				s.logger.Warn("screen_touch_failed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "error", err)
			}
		}
	}()
	videoPackets := 0
	configPackets := 0
	keyFrames := 0
	for {
		packet, config, keyFrame, err := session.ReadPacket()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
				s.logger.Info("screen_stream_closed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "videoPackets", videoPackets, "configPackets", configPackets, "keyFrames", keyFrames)
			} else {
				s.logger.Warn("screen_stream_failed", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "videoPackets", videoPackets, "configPackets", configPackets, "keyFrames", keyFrames, "error", err)
			}
			return
		}
		if packet == nil {
			continue
		}
		messageType := byte(0)
		if config {
			messageType = 1
			configPackets++
			if configPackets == 1 {
				s.logger.Info("screen_codec_config_forwarded", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "bytes", len(packet))
			}
		} else if keyFrame {
			messageType = 2
			keyFrames++
			videoPackets++
			if keyFrames == 1 {
				s.logger.Info("screen_first_keyframe_forwarded", "traceId", writer.Header().Get("X-Request-ID"), "deviceId", request.PathValue("id"), "bytes", len(packet))
			}
		} else {
			videoPackets++
		}
		if err := connection.WriteMessage(websocket.BinaryMessage, append([]byte{messageType}, packet...)); err != nil {
			s.logger.Info("screen_websocket_closed", "traceId", writer.Header().Get("X-Request-ID"), "error", err)
			return
		}
	}
}

func screenFrameRate(requested string, configured int) int {
	if configured < 1 || configured > 60 {
		configured = 30
	}
	if strings.TrimSpace(requested) == "" {
		return configured
	}
	fps, err := strconv.Atoi(requested)
	if err != nil || fps < 1 {
		return configured
	}
	if fps > 60 {
		return 60
	}
	return fps
}

func (s *Server) terminal(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Args []string `json:"args"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	if len(body.Args) == 0 || len(body.Args) > 128 {
		writeError(writer, http.StatusBadRequest, apperror.New("TERMINAL_ARGS_INVALID", "终端参数数组无效", "terminal", true))
		return
	}
	output, err := s.devices.Exec(request.Context(), device.DeviceArgs(request.PathValue("id"), body.Args...))
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, output)
}

func (s *Server) packages(writer http.ResponseWriter, request *http.Request) {
	packages, err := s.devices.Packages(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, packages)
}

func (s *Server) packageAction(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Operation string `json:"operation"`
		Package   string `json:"package"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	if !packagePattern.MatchString(body.Package) {
		writeError(writer, http.StatusBadRequest, apperror.New("PACKAGE_NAME_INVALID", "应用包名无效", "device.packages", true))
		return
	}
	var args []string
	switch body.Operation {
	case "open":
		args = []string{"shell", "monkey", "-p", body.Package, "1"}
	case "stop":
		args = []string{"shell", "am", "force-stop", body.Package}
	case "clear":
		args = []string{"shell", "pm", "clear", body.Package}
	case "uninstall":
		args = []string{"uninstall", body.Package}
	default:
		writeError(writer, http.StatusBadRequest, apperror.New("PACKAGE_ACTION_INVALID", "不支持的应用操作", "device.packages", true))
		return
	}
	if _, err := s.devices.Exec(request.Context(), device.DeviceArgs(request.PathValue("id"), args...)); err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, map[string]bool{"accepted": true})
}

func (s *Server) installPackage(writer http.ResponseWriter, request *http.Request) {
	file, header, err := request.FormFile("file")
	if err != nil {
		writeError(writer, http.StatusBadRequest, apperror.Wrap("APK_UPLOAD_INVALID", "请选择 APK 文件", "device.packages", true, err))
		return
	}
	defer file.Close()
	if !strings.EqualFold(filepath.Ext(header.Filename), ".apk") {
		writeError(writer, http.StatusBadRequest, apperror.New("APK_FILE_INVALID", "上传文件必须是 APK", "device.packages", true))
		return
	}
	temporary, err := os.CreateTemp(filepath.Join(s.config.DataDir, "uploads"), "package-*.apk")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	path := temporary.Name()
	defer os.Remove(path)
	defer func() {
		if err := temporary.Close(); err != nil {
			s.logger.Warn("apk_upload_temp_close_failed", "traceId", writer.Header().Get("X-Request-ID"), "error", err)
		}
	}()
	if _, err := io.Copy(temporary, io.LimitReader(file, 1024*1024*1024)); err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	if _, err := s.devices.Exec(request.Context(), device.DeviceArgs(request.PathValue("id"), "install", "-r", path)); err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusCreated, map[string]bool{"installed": true})
}

func (s *Server) files(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Query().Get("path")
	if path == "" {
		path = "/sdcard"
	}
	if err := device.ValidateRemotePath(path); err != nil {
		writeError(writer, http.StatusBadRequest, apperror.Wrap("REMOTE_PATH_INVALID", "设备路径无效", "device.files", true, err))
		return
	}
	entries, err := s.devices.ListFiles(request.Context(), request.PathValue("id"), path)
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, map[string]any{"path": path, "entries": entries})
}

func (s *Server) uploadFile(writer http.ResponseWriter, request *http.Request) {
	remote := request.URL.Query().Get("path")
	if err := device.ValidateUploadDirectory(remote); err != nil {
		writeError(writer, http.StatusBadRequest, apperror.Wrap("REMOTE_PATH_INVALID", "设备路径无效", "device.files", true, err))
		return
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	defer file.Close()
	temporary, err := os.CreateTemp(filepath.Join(s.config.DataDir, "uploads"), "file-*")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	path := temporary.Name()
	defer os.Remove(path)
	defer func() {
		if err := temporary.Close(); err != nil {
			s.logger.Warn("file_upload_temp_close_failed", "traceId", writer.Header().Get("X-Request-ID"), "error", err)
		}
	}()
	if _, err := io.Copy(temporary, io.LimitReader(file, 2*1024*1024*1024)); err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	remoteFile := strings.TrimRight(remote, "/") + "/" + filepath.Base(header.Filename)
	if _, err := s.devices.Exec(request.Context(), device.DeviceArgs(request.PathValue("id"), "push", path, remoteFile)); err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusCreated, map[string]string{"path": remoteFile})
}

func (s *Server) createDirectory(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	if err := s.devices.MakeDirectory(request.Context(), request.PathValue("id"), body.Path); err != nil {
		writeError(writer, http.StatusBadRequest, apperror.Wrap("REMOTE_PATH_INVALID", "目录路径无效", "device.files", true, err))
		return
	}
	writeData(writer, http.StatusCreated, map[string]string{"path": body.Path})
}

func (s *Server) deleteFile(writer http.ResponseWriter, request *http.Request) {
	remote := request.URL.Query().Get("path")
	if err := s.devices.RemoveFile(request.Context(), request.PathValue("id"), remote); err != nil {
		writeError(writer, http.StatusBadRequest, apperror.Wrap("REMOTE_PATH_INVALID", "文件路径无效", "device.files", true, err))
		return
	}
	writeData(writer, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) downloadFile(writer http.ResponseWriter, request *http.Request) {
	remote := request.URL.Query().Get("path")
	if err := device.ValidateRemotePath(remote); err != nil {
		writeError(writer, http.StatusBadRequest, apperror.Wrap("REMOTE_PATH_INVALID", "设备路径无效", "device.files", true, err))
		return
	}
	temporary, err := os.CreateTemp(filepath.Join(s.config.DataDir, "uploads"), "download-*")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	path := temporary.Name()
	if err := temporary.Close(); err != nil {
		writeError(writer, http.StatusInternalServerError, apperror.Wrap("FILE_DOWNLOAD_TEMP_CLOSE_FAILED", "下载临时文件准备失败", "device.files", true, err))
		return
	}
	defer os.Remove(path)
	if _, err := s.devices.Exec(request.Context(), device.DeviceArgs(request.PathValue("id"), "pull", remote, path)); err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(remote)))
	http.ServeFile(writer, request, path)
}

func (s *Server) capabilities(writer http.ResponseWriter, request *http.Request) {
	access, err := s.devices.CompanionCapabilities(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writer.Header().Set("X-ADBControl-Companion-Transport", access.Transport)
	if access.Transport == device.CompanionTransportADBBroadcast {
		s.logger.Info("companion_capability_catalog_fallback", "deviceId", request.PathValue("id"), "transport", access.Transport)
	}
	writeData(writer, http.StatusOK, access.Items)
}

func (s *Server) permissions(writer http.ResponseWriter, request *http.Request) {
	access, err := s.devices.CompanionPermissions(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writer.Header().Set("X-ADBControl-Companion-Transport", access.Transport)
	if access.Transport == device.CompanionTransportADBBroadcast {
		s.logger.Info("companion_permission_matrix_unavailable", "deviceId", request.PathValue("id"), "transport", access.Transport)
	}
	writeData(writer, http.StatusOK, access.Items)
}

func (s *Server) invokeCapability(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		CapabilityID string         `json:"capabilityId"`
		Operation    string         `json:"operation"`
		Args         map[string]any `json:"args"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	result, transport, err := s.devices.InvokeCompanionCapability(request.Context(), request.PathValue("id"), body.CapabilityID, body.Operation, body.Args)
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writer.Header().Set("X-ADBControl-Companion-Transport", transport)
	if transport == device.CompanionTransportADBBroadcast {
		s.logger.Info("companion_invoke_adb_fallback", "deviceId", request.PathValue("id"), "capabilityId", body.CapabilityID, "operation", body.Operation)
	}
	writeData(writer, http.StatusOK, result)
}

func (s *Server) coreResult(writer http.ResponseWriter, request *http.Request, method string, params any) {
	var result any
	if err := s.devices.Core().Call(request.Context(), method, params, &result); err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, result)
}

func (s *Server) companionStatus(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := withTimeout(request, 8*time.Second)
	defer cancel()
	status, err := s.devices.CompanionStatus(ctx, request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeData(writer, http.StatusOK, status)
}

func (s *Server) installCompanion(writer http.ResponseWriter, request *http.Request) {
	result, err := s.devices.ForceInstallConfiguredCompanion(request.Context(), request.PathValue("id"), "manual")
	if err != nil {
		writeError(writer, companionUpgradeHTTPStatus(err), err)
		return
	}
	writeData(writer, http.StatusOK, result)
}

func (s *Server) ensureCompanion(writer http.ResponseWriter, request *http.Request) {
	result, err := s.devices.CheckConfiguredCompanion(request.Context(), request.PathValue("id"), "web-preflight")
	if err != nil {
		writeError(writer, companionUpgradeHTTPStatus(err), err)
		return
	}
	writeData(writer, http.StatusOK, result)
}

func companionUpgradeHTTPStatus(err error) int {
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		return http.StatusBadGateway
	}
	switch appErr.ErrorCode {
	case "COMPANION_APK_MISSING":
		return http.StatusServiceUnavailable
	case "COMPANION_SIGNATURE_MISMATCH":
		return http.StatusConflict
	default:
		return http.StatusBadGateway
	}
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func withTimeout(request *http.Request, duration time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(request.Context(), duration)
}
