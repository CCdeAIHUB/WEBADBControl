package apperror

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// Error is the stable failure contract shared by Core, Go and the Vue client.
type Error struct {
	ErrorCode   string `json:"errorCode"`
	Message     string `json:"message"`
	Module      string `json:"module"`
	Recoverable bool   `json:"recoverable"`
	Cause       string `json:"cause,omitempty"`
	Suggestion  string `json:"suggestion,omitempty"`
	TraceID     string `json:"traceId"`
}

func New(code, message, module string, recoverable bool) *Error {
	return &Error{ErrorCode: code, Message: message, Module: module, Recoverable: recoverable, TraceID: newTraceID()}
}

func Wrap(code, message, module string, recoverable bool, cause error) *Error {
	err := New(code, message, module, recoverable)
	if cause != nil {
		err.Cause = cause.Error()
	}
	return err
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.ErrorCode, e.Message) }

func (e *Error) WithSuggestion(suggestion string) *Error {
	e.Suggestion = suggestion
	return e
}

func (e *Error) WithTraceID(traceID string) *Error {
	if traceID != "" {
		e.TraceID = traceID
	}
	return e
}

func newTraceID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "trace-unavailable"
	}
	return hex.EncodeToString(buffer)
}
