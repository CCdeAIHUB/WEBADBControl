package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
)

type apiConnectionCaller struct{ output device.CommandOutput }

func (c apiConnectionCaller) Call(_ context.Context, _ string, _ any, target any) error {
	*(target.(*device.CommandOutput)) = c.output
	return nil
}

type apiQRCaller struct {
	method string
	params any
}

func (c *apiQRCaller) Call(_ context.Context, method string, params any, target any) error {
	c.method, c.params = method, params
	switch method {
	case "adb.wifi.qr.create":
		*(target.(*device.WirelessPairingQR)) = device.WirelessPairingQR{SessionID: "qr-session", ServiceName: "studio-abc", QRSVG: "<svg/>", MimeType: "image/svg+xml", ExpiresAt: 1_700_000_000}
	case "adb.wifi.qr.pair":
		*(target.(*device.WirelessQRPairingResult)) = device.WirelessQRPairingResult{Paired: true, ServiceName: "studio-abc", Endpoint: "192.168.3.20:37123"}
	}
	return nil
}

func TestQRPairingHandlersUseOpaqueCoreSession(t *testing.T) {
	// 场景：Web 只能传递 Core 生成的会话 ID，二维码秘密不得进入 HTTP 响应或请求参数。
	caller := &apiQRCaller{}
	server := &Server{devices: device.NewService(caller), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/wireless/qr-pairings", nil)
	createResponse := httptest.NewRecorder()
	server.createQRPairing(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated || caller.method != "adb.wifi.qr.create" {
		t.Fatalf("create status=%d method=%q body=%s", createResponse.Code, caller.method, createResponse.Body.String())
	}
	if bytes.Contains(createResponse.Body.Bytes(), []byte("password")) {
		t.Fatalf("QR response leaked a password: %s", createResponse.Body.String())
	}

	pairRequest := httptest.NewRequest(http.MethodPost, "/api/v1/wireless/qr-pairings/qr-session/pair", nil)
	pairRequest.SetPathValue("sessionId", "qr-session")
	pairResponse := httptest.NewRecorder()
	server.pairQRDevice(pairResponse, pairRequest)
	if pairResponse.Code != http.StatusOK || caller.method != "adb.wifi.qr.pair" {
		t.Fatalf("pair status=%d method=%q body=%s", pairResponse.Code, caller.method, pairResponse.Body.String())
	}
	params := caller.params.(map[string]any)
	if params["sessionId"] != "qr-session" || len(params) != 1 {
		t.Fatalf("unexpected QR pair params: %#v", params)
	}
}

func TestPairHandlerReturnsBadRequestForInvalidEndpoint(t *testing.T) {
	// 场景：无效配对地址属于客户端输入错误，API 不得伪装成 502 上游故障。
	server := &Server{devices: device.NewService(apiConnectionCaller{})}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/pair", bytes.NewBufferString(`{"endpoint":"192.168.3.20","code":"123456"}`))
	response := httptest.NewRecorder()

	server.pairDevice(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
	}
}

func TestPairHandlerExposesUnsupportedADBTrace(t *testing.T) {
	// 场景：旧 ADB 缺少 pair 命令时，API 必须返回服务能力错误和可关联 traceId。
	server := &Server{devices: device.NewService(apiConnectionCaller{output: device.CommandOutput{ExitCode: 1, Stderr: "adb: unknown command pair"}})}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/pair", bytes.NewBufferString(`{"endpoint":"192.168.3.20:37123","code":"123456"}`))
	response := httptest.NewRecorder()

	server.pairDevice(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Error struct{ ErrorCode, TraceID string } `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.ErrorCode != "ADB_PAIR_UNSUPPORTED" || body.Error.TraceID == "" || response.Header().Get("X-Request-ID") != body.Error.TraceID {
		t.Fatalf("unexpected error contract: body=%s header=%q", response.Body.String(), response.Header().Get("X-Request-ID"))
	}
}

func TestLoggingIncludesHTTPStatus(t *testing.T) {
	// 场景：关键请求日志必须包含状态码和 traceId，排查配对失败时不能只看到路径和耗时。
	var output bytes.Buffer
	server := &Server{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	handler := server.logging(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Request-ID", "trace-test")
		writeError(writer, http.StatusTeapot, bytes.ErrTooLarge)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/devices/pair", nil))

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["status"] != float64(http.StatusTeapot) || entry["traceId"] == "" {
		t.Fatalf("log entry missing status/trace: %s", output.String())
	}
}
