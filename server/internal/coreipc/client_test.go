package coreipc

import (
	"context"
	"encoding/json"
	"testing"
)

type recordingTransport struct {
	request Request
}

func (t *recordingTransport) RoundTrip(_ context.Context, request Request) (Response, error) {
	t.request = request
	return Response{ID: request.ID, OK: true, Result: json.RawMessage(`{"exitCode":0,"stdout":"ok","stderr":""}`)}, nil
}

func TestClientPreservesCoreMethodAndArgumentArray(t *testing.T) {
	// 场景：Go 网络层只能适配原 Core 契约，不能把 ADB 参数重新拼成主机 shell 文本。
	transport := &recordingTransport{}
	client := NewClient(transport)
	var output struct {
		Stdout string `json:"stdout"`
	}
	if err := client.Call(context.Background(), "adb.exec", map[string]any{"args": []string{"-s", "device-1", "shell", "getprop"}}, &output); err != nil {
		t.Fatal(err)
	}

	if transport.request.Method != "adb.exec" {
		t.Fatalf("unexpected method %q", transport.request.Method)
	}
	params, ok := transport.request.Params.(map[string]any)
	if !ok {
		t.Fatalf("unexpected params type %T", transport.request.Params)
	}
	args, ok := params["args"].([]string)
	if !ok || len(args) != 4 || args[1] != "device-1" {
		t.Fatalf("argument array was not preserved: %#v", params["args"])
	}
	if output.Stdout != "ok" {
		t.Fatalf("unexpected decoded output %q", output.Stdout)
	}
}

func TestClientRejectsMismatchedResponseID(t *testing.T) {
	// 场景：Core 响应关联错误时必须显式失败，不能把其他请求结果交给当前调用方。
	client := NewClient(TransportFunc(func(_ context.Context, request Request) (Response, error) {
		return Response{ID: "another-request", OK: true, Result: json.RawMessage(`{}`)}, nil
	}))
	if err := client.Call(context.Background(), "device.list", map[string]any{}, nil); err == nil {
		t.Fatal("expected response correlation error")
	}
}
