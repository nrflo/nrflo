package socket

import (
	"encoding/json"
	"fmt"
)

// Request represents a JSON-RPC style request from CLI to server
type Request struct {
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	Project string          `json:"project"`
}

// Response represents a JSON-RPC style response from server to CLI
type Response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *ErrorInfo      `json:"error,omitempty"`
}

// StreamChunk represents a streaming response chunk (for agent spawn)
type StreamChunk struct {
	ID     string       `json:"id"`
	Stream *StreamEvent `json:"stream,omitempty"`
}

// StreamEvent represents a streaming event
type StreamEvent struct {
	Type    string          `json:"type"` // "progress", "output", "complete", "error"
	Payload json.RawMessage `json:"payload"`
}

// ErrorInfo represents an error in a response
type ErrorInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error codes
const (
	ErrCodeParse          = -32700 // Invalid JSON
	ErrCodeInvalidRequest = -32600 // Invalid request
	ErrCodeMethodNotFound = -32601 // Method not found
	ErrCodeInvalidParams  = -32602 // Invalid params
	ErrCodeInternal       = -32603 // Internal error
	ErrCodeNotFound       = -32604 // Resource not found
	ErrCodeConflict       = -32605 // Conflict (e.g., already exists)
	ErrCodeValidation     = -32606 // Validation error
)

// Error implements the error interface for ErrorInfo
func (e *ErrorInfo) Error() string {
	return fmt.Sprintf("code %d: %s", e.Code, e.Message)
}

// NewError creates a new ErrorInfo
func NewError(code int, message string) *ErrorInfo {
	return &ErrorInfo{Code: code, Message: message}
}

// NewParseError creates a parse error
func NewParseError(message string) *ErrorInfo {
	return NewError(ErrCodeParse, message)
}

// NewInvalidRequestError creates an invalid request error
func NewInvalidRequestError(message string) *ErrorInfo {
	return NewError(ErrCodeInvalidRequest, message)
}

// NewMethodNotFoundError creates a method not found error
func NewMethodNotFoundError(method string) *ErrorInfo {
	return NewError(ErrCodeMethodNotFound, fmt.Sprintf("method not found: %s", method))
}

// NewInvalidParamsError creates an invalid params error
func NewInvalidParamsError(message string) *ErrorInfo {
	return NewError(ErrCodeInvalidParams, message)
}

// NewInternalError creates an internal error
func NewInternalError(message string) *ErrorInfo {
	return NewError(ErrCodeInternal, message)
}

// NewNotFoundError creates a not found error
func NewNotFoundError(message string) *ErrorInfo {
	return NewError(ErrCodeNotFound, message)
}

// NewConflictError creates a conflict error
func NewConflictError(message string) *ErrorInfo {
	return NewError(ErrCodeConflict, message)
}

// NewValidationError creates a validation error
func NewValidationError(message string) *ErrorInfo {
	return NewError(ErrCodeValidation, message)
}

// MakeResponse creates a successful response
func MakeResponse(id string, result interface{}) Response {
	data, _ := json.Marshal(result)
	return Response{
		ID:     id,
		Result: data,
	}
}

// MakeErrorResponse creates an error response
func MakeErrorResponse(id string, err *ErrorInfo) Response {
	return Response{
		ID:    id,
		Error: err,
	}
}

// Progress payload for streaming
type ProgressPayload struct {
	Message string `json:"message"`
	Phase   string `json:"phase,omitempty"`
}

// OutputPayload for streaming output
type OutputPayload struct {
	Line   string `json:"line"`
	Source string `json:"source,omitempty"` // "stdout", "stderr"
}

// CompletePayload for streaming completion
type CompletePayload struct {
	Result   string `json:"result"` // "pass", "fail"
	ExitCode int    `json:"exit_code,omitempty"`
	Elapsed  string `json:"elapsed,omitempty"`
}

// MakeStreamProgress creates a progress stream chunk
func MakeStreamProgress(id, message, phase string) StreamChunk {
	payload, _ := json.Marshal(ProgressPayload{Message: message, Phase: phase})
	return StreamChunk{
		ID: id,
		Stream: &StreamEvent{
			Type:    "progress",
			Payload: payload,
		},
	}
}

// MakeStreamOutput creates an output stream chunk
func MakeStreamOutput(id, line, source string) StreamChunk {
	payload, _ := json.Marshal(OutputPayload{Line: line, Source: source})
	return StreamChunk{
		ID: id,
		Stream: &StreamEvent{
			Type:    "output",
			Payload: payload,
		},
	}
}

// MakeStreamComplete creates a complete stream chunk
func MakeStreamComplete(id, result string, exitCode int, elapsed string) StreamChunk {
	payload, _ := json.Marshal(CompletePayload{Result: result, ExitCode: exitCode, Elapsed: elapsed})
	return StreamChunk{
		ID: id,
		Stream: &StreamEvent{
			Type:    "complete",
			Payload: payload,
		},
	}
}

// MakeStreamError creates an error stream chunk
func MakeStreamError(id string, err *ErrorInfo) StreamChunk {
	payload, _ := json.Marshal(err)
	return StreamChunk{
		ID: id,
		Stream: &StreamEvent{
			Type:    "error",
			Payload: payload,
		},
	}
}
