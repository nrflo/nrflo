package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
)

// runMCPStdioLoop runs a newline-delimited JSON-RPC 2.0 loop over in/out,
// dispatching each request through fn. fn returns nil for notifications (no
// reply). Used by the session-bound `agent mcp` bridge; the token-authed
// `agent mcp-external` proxy uses runMCPStdioLoopWithCancel instead.
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

// idKey is a stable map key for a JSON-RPC id (string or number) so an inbound
// notifications/cancelled can be matched to the in-flight request it cancels.
func idKey(id interface{}) string {
	b, _ := json.Marshal(id)
	return string(b)
}

// runMCPStdioLoopWithCancel is a newline-delimited JSON-RPC 2.0 loop that threads
// a per-request context into fn and honors MCP `notifications/cancelled` (and
// stdin EOF) by cancelling the matching in-flight request's context. The external
// proxy uses it so a cancelled/timed-out blocking tool call can
// stop its server-side run instead of orphaning it. Requests are dispatched in
// their own goroutines (the read loop never blocks, so a cancellation arriving
// mid-call is always processed); writes are serialized.
func runMCPStdioLoopWithCancel(parent context.Context, in io.Reader, out io.Writer, fn func(context.Context, mcpRequest) *mcpResponse) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	writer := bufio.NewWriter(out)

	var wmu sync.Mutex // serializes response writes across dispatch goroutines
	emit := func(resp *mcpResponse) {
		wmu.Lock()
		defer wmu.Unlock()
		data, err := json.Marshal(resp)
		if err != nil {
			return
		}
		_, _ = writer.Write(data)
		_ = writer.WriteByte('\n')
		_ = writer.Flush()
	}

	var mu sync.Mutex
	inflight := map[string]context.CancelFunc{}
	var wg sync.WaitGroup

	for scanner.Scan() {
		var req mcpRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			emit(makeMCPError(nil, -32700, "parse error: "+err.Error()))
			continue
		}
		if req.Method == "notifications/cancelled" {
			var p struct {
				RequestID interface{} `json:"requestId"`
			}
			_ = json.Unmarshal(req.Params, &p)
			mu.Lock()
			if cancel := inflight[idKey(p.RequestID)]; cancel != nil {
				cancel()
			}
			mu.Unlock()
			continue
		}
		ctx, cancel := context.WithCancel(parent)
		key := idKey(req.ID)
		mu.Lock()
		inflight[key] = cancel
		mu.Unlock()
		wg.Add(1)
		go func(req mcpRequest, ctx context.Context, cancel context.CancelFunc, key string) {
			defer wg.Done()
			defer func() {
				mu.Lock()
				delete(inflight, key)
				mu.Unlock()
				cancel()
			}()
			if resp := fn(ctx, req); resp != nil {
				emit(resp)
			}
		}(req, ctx, cancel, key)
	}

	// stdin EOF (e.g. the client killed the proxy): cancel every in-flight request
	// so each can stop its server-side run, then wait for those stops to fire.
	mu.Lock()
	for _, cancel := range inflight {
		cancel()
	}
	mu.Unlock()
	wg.Wait()
	return scanner.Err()
}
