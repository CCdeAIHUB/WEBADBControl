package device

import (
	"context"
	"testing"
)

func TestConnectRejectsMissingPortBeforeADB(t *testing.T) {
	// 场景：直接连接也必须校验完整 host:port，不能把无端口地址交给 ADB 后再返回泛化错误。
	caller := &connectionCaller{}
	err := NewService(caller).Connect(context.Background(), "192.168.3.20")
	if codeOf(err) != "ADB_ENDPOINT_INVALID" || len(caller.calls) != 0 {
		t.Fatalf("error=%v calls=%#v", err, caller.calls)
	}
}
