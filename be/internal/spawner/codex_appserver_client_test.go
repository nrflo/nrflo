package spawner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type sentReq struct {
	id     int64
	method string
}

// testConn wires an appServerClient over in-memory pipes. The client's stdin
// is drained into `sent` (so writes never block), and `stdoutW` lets the test
// feed inbound JSON-RPC lines.
type testConn struct {
	client  *appServerClient
	stdoutW *io.PipeWriter
	sent    chan sentReq
}

func newTestConn(t *testing.T) *testConn {
	t.Helper()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	c := newAppServerClient(stdinW, stdoutR, nil)
	sent := make(chan sentReq, 256)
	go func() {
		r := bufio.NewReader(stdinR)
		for {
			line, err := r.ReadBytes('\n')
			if len(line) > 0 {
				var o struct {
					ID     int64  `json:"id"`
					Method string `json:"method"`
				}
				if json.Unmarshal(line, &o) == nil && o.Method != "" {
					sent <- sentReq{id: o.ID, method: o.Method}
				}
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		c.close()
		_ = stdoutW.Close()
		_ = stdinR.Close()
	})
	return &testConn{client: c, stdoutW: stdoutW, sent: sent}
}

func (tc *testConn) feed(line string) {
	_, _ = tc.stdoutW.Write([]byte(line + "\n"))
}

func TestAppServerClient_CallResponse(t *testing.T) {
	tc := newTestConn(t)
	type result struct {
		raw json.RawMessage
		err error
	}
	resCh := make(chan result, 1)
	go func() {
		raw, err := tc.client.call(context.Background(), "thread/start", map[string]any{"model": "m"})
		resCh <- result{raw, err}
	}()

	var req sentReq
	select {
	case req = <-tc.sent:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not send request")
	}
	if req.method != "thread/start" {
		t.Fatalf("sent method = %q", req.method)
	}
	tc.feed(fmt.Sprintf(`{"id":%d,"result":{"thread":{"id":"T1"}}}`, req.id))

	select {
	case r := <-resCh:
		if r.err != nil {
			t.Fatalf("call err: %v", r.err)
		}
		var got struct {
			Thread struct{ ID string } `json:"thread"`
		}
		_ = json.Unmarshal(r.raw, &got)
		if got.Thread.ID != "T1" {
			t.Errorf("thread id = %q, want T1", got.Thread.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call did not return")
	}
}

func TestAppServerClient_NotificationAndServerRequest(t *testing.T) {
	tc := newTestConn(t)
	tc.feed(`{"method":"turn/started","params":{"threadId":"T1"}}`)
	tc.feed(`{"id":42,"method":"execCommandApproval","params":{}}`)

	select {
	case n := <-tc.client.notifyCh:
		if n.Method != "turn/started" {
			t.Errorf("notification method = %q", n.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no notification delivered")
	}
	select {
	case r := <-tc.client.reqCh:
		if r.Method != "execCommandApproval" || r.ID == nil {
			t.Errorf("server request = %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no server request delivered")
	}
}

func TestAppServerClient_PartialTrailingLine(t *testing.T) {
	tc := newTestConn(t)
	// A complete notification followed by a partial line, then EOF.
	_, _ = tc.stdoutW.Write([]byte(`{"method":"turn/started","params":{}}` + "\n" + `{"method":"turn/comp`))
	select {
	case n := <-tc.client.notifyCh:
		if n.Method != "turn/started" {
			t.Errorf("method = %q", n.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("complete line not delivered")
	}
	_ = tc.stdoutW.Close() // EOF with partial buffered — must not produce a notification
	select {
	case n := <-tc.client.notifyCh:
		t.Errorf("partial line wrongly decoded: %+v", n)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAppServerClient_OversizedLine(t *testing.T) {
	tc := newTestConn(t)
	big := strings.Repeat("x", 200000)
	tc.feed(fmt.Sprintf(`{"method":"item/completed","params":{"item":{"type":"commandExecution","command":"c","aggregatedOutput":%q}}}`, big))
	select {
	case n := <-tc.client.notifyCh:
		var p struct {
			Item appServerItem `json:"item"`
		}
		if json.Unmarshal(n.Params, &p) != nil || len(p.Item.AggregatedOutput) != 200000 {
			t.Errorf("oversized line not reassembled: out len=%d", len(p.Item.AggregatedOutput))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("oversized line not delivered")
	}
}

func TestAppServerClient_ConcurrentCalls(t *testing.T) {
	tc := newTestConn(t)
	const n = 20
	// Responder: echo each request id back with a unique result.
	go func() {
		for i := 0; i < n; i++ {
			req := <-tc.sent
			tc.feed(fmt.Sprintf(`{"id":%d,"result":{"echo":%d}}`, req.id, req.id))
		}
	}()
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := tc.client.call(context.Background(), "ping", nil)
			if err != nil {
				errs <- err
				return
			}
			var got struct {
				Echo int64 `json:"echo"`
			}
			_ = json.Unmarshal(raw, &got)
			// The response echoes the id we were assigned; just assert it parsed.
			if got.Echo == 0 {
				errs <- fmt.Errorf("empty echo")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent call err: %v", err)
	}
}

func TestAppServerClient_CallUnblocksOnClose(t *testing.T) {
	tc := newTestConn(t)
	resCh := make(chan error, 1)
	go func() {
		_, err := tc.client.call(context.Background(), "thread/start", nil)
		resCh <- err
	}()
	<-tc.sent // ensure the request was sent
	tc.client.close()
	select {
	case err := <-resCh:
		if err == nil {
			t.Error("call should error after close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call did not unblock on close")
	}
}
