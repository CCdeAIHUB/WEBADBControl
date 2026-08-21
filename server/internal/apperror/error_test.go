package apperror

import (
	"encoding/json"
	"testing"
)

func TestErrorContractIncludesStableDiagnostics(t *testing.T) {
	// 场景：任何关键失败都必须保留稳定错误码、模块、可恢复性和追踪标识。
	err := New("DEVICE_OFFLINE", "设备未连接", "device.service", true).WithSuggestion("请重新连接设备")
	data, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}

	var got map[string]any
	if unmarshalErr := json.Unmarshal(data, &got); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	for _, key := range []string{"errorCode", "message", "module", "recoverable", "traceId"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing required error field %q", key)
		}
	}
	if got["errorCode"] != "DEVICE_OFFLINE" || got["recoverable"] != true {
		t.Fatalf("unexpected error contract: %#v", got)
	}
}
