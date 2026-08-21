package coreipc

import "encoding/json"

type Request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type Response struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *CoreError      `json:"error,omitempty"`
}

type CoreError struct {
	ErrorCode   string `json:"errorCode"`
	Message     string `json:"message"`
	Module      string `json:"module"`
	Recoverable bool   `json:"recoverable"`
	Cause       string `json:"cause,omitempty"`
	Suggestion  string `json:"suggestion,omitempty"`
	TraceID     string `json:"traceId,omitempty"`
}
