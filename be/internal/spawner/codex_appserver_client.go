package spawner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// appServerClient is a minimal newline-delimited JSON-RPC stdio client for
// `codex app-server`. It is transport-only — no spawner/processInfo dependency
// — so it is unit-testable against an in-memory pipe. The codex app-server
// speaks JSON-RPC where each message is a single JSON object on its own line
// (one object per `\n`). Messages have no `jsonrpc` field on the wire; we still
// send one (accepted) and demux purely on id/method/result presence.
//
// Demux rules (readLoop):
//   - id present, method empty  → RESPONSE to a prior call → routed to pending[id]
//   - id present, method set    → SERVER REQUEST (e.g. approval) → reqCh
//   - id absent,  method set     → NOTIFICATION (events) → notifyCh
type appServerClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	enc     *json.Encoder
	writeMu sync.Mutex
	nextID  atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan rpcEnvelope

	notifyCh chan rpcEnvelope
	reqCh    chan rpcEnvelope

	closed    chan struct{}
	closeOnce sync.Once
	readErr   atomic.Value // error, set when readLoop exits
}

// rpcEnvelope is the decoded shape of one inbound wire object. Fields are
// RawMessage so we can decode lazily and demux by shape before interpreting.
type rpcEnvelope struct {
	ID     *json.RawMessage `json:"id"`
	Method string           `json:"method"`
	Params json.RawMessage  `json:"params"`
	Result json.RawMessage  `json:"result"`
	Error  *rpcError        `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcOut is one outbound request/notification. id==0 (notification) is omitted.
type rpcOut struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

var errAppServerClosed = errors.New("codex app-server connection closed")

// appServerArgs returns the `codex app-server` argv with native delegation
// blocked via codexAgentsArgs (`-c agents.enabled=false`), placed first so the
// security-critical override sits adjacent to the subcommand. Kept pure (no
// exec) so it is unit-testable without running the codex binary.
func appServerArgs() []string {
	args := []string{"app-server"}
	args = append(args, codexAgentsArgs()...)
	args = append(args, codexProjectDocArgs()...)
	args = append(args, codexAutoCompactArgs()...)
	return args
}

// newAppServerClient wires a client over the given stdin/stdout and starts the
// reader goroutine. Used directly by tests (in-memory pipes); production goes
// through startAppServer.
func newAppServerClient(stdin io.WriteCloser, stdout io.Reader, cmd *exec.Cmd) *appServerClient {
	c := &appServerClient{
		cmd:      cmd,
		stdin:    stdin,
		enc:      json.NewEncoder(stdin),
		pending:  make(map[int64]chan rpcEnvelope),
		notifyCh: make(chan rpcEnvelope, 2048),
		reqCh:    make(chan rpcEnvelope, 64),
		closed:   make(chan struct{}),
	}
	go c.readLoop(stdout)
	return c
}

// dialAppServer is startAppServer by default; the console engine dials
// through this var so unit tests can wire a fake app-server over in-memory
// pipes without exec'ing codex.
var dialAppServer = startAppServer

// startAppServer spawns `codex app-server` with the given env + workDir and
// returns a wired client. The process is bound to ctx (exec.CommandContext), so
// cancelling ctx kills it.
func startAppServer(ctx context.Context, env []string, workDir string) (*appServerClient, error) {
	cmd := exec.CommandContext(ctx, "codex", appServerArgs()...)
	cmd.Dir = workDir
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	return newAppServerClient(stdin, stdout, cmd), nil
}

// readLoop reads newline-delimited JSON objects and demuxes them. bufio.Reader.
// ReadBytes('\n') returns a complete line regardless of length (it grows as
// needed), so oversized commandExecution output is handled without ErrBufferFull.
func (c *appServerClient) readLoop(stdout io.Reader) {
	r := bufio.NewReaderSize(stdout, 1<<16)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var env rpcEnvelope
			if json.Unmarshal(line, &env) == nil {
				c.dispatch(env)
			}
		}
		if err != nil {
			c.fail(err)
			return
		}
	}
}

func (c *appServerClient) dispatch(env rpcEnvelope) {
	switch {
	case env.ID != nil && env.Method == "":
		// Response to a prior call.
		id := parseRPCID(*env.ID)
		c.mu.Lock()
		ch := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if ch != nil {
			ch <- env
		}
	case env.ID != nil && env.Method != "":
		c.send(c.reqCh, env)
	case env.Method != "":
		c.send(c.notifyCh, env)
	}
}

// send delivers to a demux channel, unblocking on close. The channels are
// large-buffered and the backend drains continuously, so this rarely blocks.
func (c *appServerClient) send(ch chan rpcEnvelope, env rpcEnvelope) {
	select {
	case ch <- env:
	case <-c.closed:
	}
}

func (c *appServerClient) fail(err error) {
	if err == io.EOF {
		err = errAppServerClosed
	}
	c.readErr.Store(err)
	c.closeOnce.Do(func() { close(c.closed) })
	// Unblock any in-flight calls.
	c.mu.Lock()
	for id, ch := range c.pending {
		delete(c.pending, id)
		close(ch)
	}
	c.mu.Unlock()
}

// call sends a request and blocks for the correlated response (or ctx/closed).
func (c *appServerClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan rpcEnvelope, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.write(rpcOut{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, c.err()
	case env, ok := <-ch:
		if !ok {
			return nil, c.err()
		}
		if env.Error != nil {
			return nil, fmt.Errorf("app-server %s: rpc error %d: %s", method, env.Error.Code, env.Error.Message)
		}
		return env.Result, nil
	}
}

// notify sends a fire-and-forget notification (no id, no response).
func (c *appServerClient) notify(method string, params any) error {
	return c.write(rpcOut{JSONRPC: "2.0", Method: method, Params: params})
}

// reply answers a server→client request, echoing its raw id.
func (c *appServerClient) reply(rawID json.RawMessage, result any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.closed:
		return c.err()
	default:
	}
	return c.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": rawID, "result": result})
}

// replyError answers a server->client request with a JSON-RPC error instead
// of a result, so codex is not left blocked on a request the caller does not
// implement (e.g. a console engine rejecting item/permissions/requestApproval).
func (c *appServerClient) replyError(rawID json.RawMessage, code int, msg string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.closed:
		return c.err()
	default:
	}
	return c.enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      rawID,
		"error":   map[string]any{"code": code, "message": msg},
	})
}

func (c *appServerClient) write(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.closed:
		return c.err()
	default:
	}
	return c.enc.Encode(v)
}

func (c *appServerClient) err() error {
	if v := c.readErr.Load(); v != nil {
		return v.(error)
	}
	return errAppServerClosed
}

// close closes stdin (signals EOF to app-server) and marks the client closed.
// The process is killed via the ctx passed to startAppServer; we also reap it.
func (c *appServerClient) close() {
	c.closeOnce.Do(func() { close(c.closed) })
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil {
		_ = c.cmd.Wait()
	}
}

// parseRPCID extracts an int64 id from a raw JSON id (we only ever send ints).
func parseRPCID(raw json.RawMessage) int64 {
	var i int64
	if json.Unmarshal(raw, &i) == nil {
		return i
	}
	return 0
}
