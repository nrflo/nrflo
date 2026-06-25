package cli

import (
	"bufio"
	"encoding/json"
	"io"
)

// runMCPStdioLoop runs a newline-delimited JSON-RPC 2.0 loop over in/out,
// dispatching each request through fn. fn returns nil for notifications (no
// reply). Shared by the session-bound `agent mcp` bridge and the token-authed
// `agent mcp-external` proxy.
func runMCPStdioLoop(in io.Reader, out io.Writer, fn func(mcpRequest) *mcpResponse) error {
	scanner := bufio.NewScanner(in)
	// Tool arguments can exceed the 64 KiB default token size; allow up to 8 MiB.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	writer := bufio.NewWriter(out)

	emit := func(resp *mcpResponse) error {
		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		if _, err := writer.Write(data); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
		return writer.Flush()
	}

	for scanner.Scan() {
		var req mcpRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			if werr := emit(makeMCPError(nil, -32700, "parse error: "+err.Error())); werr != nil {
				return werr
			}
			continue
		}
		resp := fn(req)
		if resp == nil {
			continue // notification — no reply
		}
		if werr := emit(resp); werr != nil {
			return werr
		}
	}
	return scanner.Err()
}
