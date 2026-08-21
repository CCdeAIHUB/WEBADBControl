package coreipc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/apperror"
)

type Transport interface {
	RoundTrip(context.Context, Request) (Response, error)
}

type TransportFunc func(context.Context, Request) (Response, error)

func (f TransportFunc) RoundTrip(ctx context.Context, request Request) (Response, error) {
	return f(ctx, request)
}

type Caller interface {
	Call(context.Context, string, any, any) error
}

type Client struct {
	transport Transport
	sequence  atomic.Uint64
}

func NewClient(transport Transport) *Client { return &Client{transport: transport} }

func (c *Client) Call(ctx context.Context, method string, params any, target any) error {
	id := fmt.Sprintf("web-%d", c.sequence.Add(1))
	response, err := c.transport.RoundTrip(ctx, Request{ID: id, Method: method, Params: params})
	if err != nil {
		return apperror.Wrap("CORE_UNAVAILABLE", "核心服务不可用", "core.ipc", true, err).
			WithSuggestion("请检查 Rust Core 是否已安装并可执行")
	}
	if response.ID != id {
		return apperror.New("CORE_RESPONSE_MISMATCH", "核心响应与请求不匹配", "core.ipc", false)
	}
	if !response.OK {
		if response.Error == nil {
			return apperror.New("CORE_INVALID_RESPONSE", "核心返回了不完整的错误响应", "core.ipc", false)
		}
		return (&apperror.Error{
			ErrorCode: response.Error.ErrorCode, Message: response.Error.Message, Module: response.Error.Module,
			Recoverable: response.Error.Recoverable, Cause: response.Error.Cause,
			Suggestion: response.Error.Suggestion, TraceID: response.Error.TraceID,
		}).WithTraceID(response.Error.TraceID)
	}
	if target == nil || len(response.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.Result, target); err != nil {
		return apperror.Wrap("CORE_RESULT_INVALID", "核心响应无法解析", "core.ipc", false, err)
	}
	return nil
}
