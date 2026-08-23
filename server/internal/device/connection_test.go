package device

import (
	"context"
	"reflect"
	"testing"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

type connectionCaller struct {
	calls  [][]string
	output CommandOutput
}

func (c *connectionCaller) Call(_ context.Context, method string, params any, target any) error {
	if method != "adb.exec" {
		return nil
	}
	args := params.(map[string]any)["args"].([]string)
	c.calls = append(c.calls, append([]string(nil), args...))
	*(target.(*CommandOutput)) = c.output
	return nil
}

func TestPairNormalizesEndpointAndCodeBeforeADB(t *testing.T) {
	// 场景：用户复制的地址和六位码可能带空格；服务端必须规范化后按独立参数传给 ADB。
	caller := &connectionCaller{}
	service := NewService(caller)

	if err := service.Pair(context.Background(), " 192.168.3.20:37123 ", " 123456 "); err != nil {
		t.Fatal(err)
	}

	want := []string{"pair", "192.168.3.20:37123", "123456"}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("pair args = %#v, want %#v", caller.calls, want)
	}
}

func TestPairRejectsInvalidEndpointBeforeADB(t *testing.T) {
	// 场景：配对地址必须包含合法主机和端口，错误输入不能进入 Core。
	for _, endpoint := range []string{"192.168.3.20", "192.168.3.20:70000", ":37123", "bad host:37123"} {
		caller := &connectionCaller{}
		err := NewService(caller).Pair(context.Background(), endpoint, "123456")
		if codeOf(err) != "ADB_ENDPOINT_INVALID" {
			t.Fatalf("endpoint %q error = %v", endpoint, err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("invalid endpoint %q reached ADB", endpoint)
		}
	}
}

func TestPairReportsUnsupportedADBInsteadOfGenericFailure(t *testing.T) {
	// 场景：发行版自带旧 ADB 不支持 pair 时，必须返回可执行的版本错误而不是泛化命令失败。
	caller := &connectionCaller{output: CommandOutput{ExitCode: 1, Stderr: "adb: unknown command pair"}}
	err := NewService(caller).Pair(context.Background(), "192.168.3.20:37123", "123456")

	if codeOf(err) != "ADB_PAIR_UNSUPPORTED" {
		t.Fatalf("error = %#v, want ADB_PAIR_UNSUPPORTED", err)
	}
}

func TestDiscoverReportsUnsupportedADBInsteadOfGenericFailure(t *testing.T) {
	// 场景：旧 ADB 不支持 mdns 时，发现接口必须指出 Platform-Tools 版本问题。
	caller := &connectionCaller{output: CommandOutput{ExitCode: 1, Stderr: "adb: unknown command mdns"}}
	_, err := NewService(caller).Discover(context.Background())

	if codeOf(err) != "ADB_MDNS_UNSUPPORTED" {
		t.Fatalf("error = %#v, want ADB_MDNS_UNSUPPORTED", err)
	}
}

func codeOf(err error) string {
	if typed, ok := err.(*apperror.Error); ok {
		return typed.ErrorCode
	}
	return ""
}
