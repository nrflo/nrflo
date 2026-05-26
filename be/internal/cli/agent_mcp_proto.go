package cli

import (
	"encoding/json"
	"fmt"
)

// mcpRequest is a JSON-RPC 2.0 request from the MCP client (e.g. Claude PTY).
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"` // string, number, or null; absent for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// mcpResponse is a JSON-RPC 2.0 response sent back to the MCP client.
type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}

// mcpError is a JSON-RPC 2.0 error object.
type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// mcpSocketCaller abstracts the socket client so tests can inject a fake.
type mcpSocketCaller interface {
	Call(method string, params map[string]interface{}) (json.RawMessage, error)
}

func makeMCPError(id interface{}, code int, msg string) *mcpResponse {
	return &mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &mcpError{Code: code, Message: msg},
	}
}

func makeMCPResult(id, result interface{}) *mcpResponse {
	return &mcpResponse{JSONRPC: "2.0", ID: id, Result: result}
}

// dispatchMCP handles a single JSON-RPC 2.0 request and returns a response.
// Returns nil for notifications (no response expected).
func dispatchMCP(req mcpRequest, sessionID, instanceID string, caller mcpSocketCaller) *mcpResponse {
	switch req.Method {
	case "initialize":
		return makeMCPResult(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": "nrflo", "version": "1.0"},
		})

	case "notifications/initialized":
		return nil // notification — no reply

	case "tools/list":
		params := map[string]interface{}{
			"session_id":  sessionID,
			"instance_id": instanceID,
		}
		raw, err := caller.Call("tools.list", params)
		if err != nil {
			return makeMCPError(req.ID, -32603, fmt.Sprintf("tools.list failed: %v", err))
		}
		var tools []json.RawMessage
		if err := json.Unmarshal(raw, &tools); err != nil {
			return makeMCPError(req.ID, -32603, fmt.Sprintf("parse tools: %v", err))
		}
		if tools == nil {
			tools = []json.RawMessage{}
		}
		return makeMCPResult(req.ID, map[string]interface{}{"tools": tools})

	case "tools/call":
		var callReq struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &callReq); err != nil {
				return makeMCPError(req.ID, -32602, fmt.Sprintf("invalid params: %v", err))
			}
		}
		if callReq.Name == "" {
			return makeMCPError(req.ID, -32602, "name is required")
		}
		input := callReq.Arguments
		if input == nil {
			input = json.RawMessage("{}")
		}
		params := map[string]interface{}{
			"session_id":  sessionID,
			"instance_id": instanceID,
			"name":        callReq.Name,
			"input":       input,
		}
		raw, err := caller.Call("tools.call", params)
		if err != nil {
			return makeMCPError(req.ID, -32603, fmt.Sprintf("tools.call failed: %v", err))
		}
		var result struct {
			Output  string `json:"output"`
			IsError bool   `json:"is_error"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return makeMCPError(req.ID, -32603, fmt.Sprintf("parse result: %v", err))
		}
		return makeMCPResult(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": result.Output},
			},
			"isError": result.IsError,
		})

	default:
		return makeMCPError(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}
