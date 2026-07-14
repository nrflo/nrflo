package spawner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAppServer is a scripted `codex app-server` double wired over in-memory
// pipes (extends the testConn pattern from codex_appserver_client_test.go): it
// auto-answers initialize/thread/start/turn/start by echoing the request id
// with a canned result, lets tests override any method's response (e.g. to
// script an rpc error), capture every outbound wire line for sequence
// assertions, and feed arbitrary inbound notification/server-request lines.
// No codex binary, no PTY, no sleeps.
type fakeAppServer struct {
	client  *appServerClient
	stdoutW *io.PipeWriter

	writeMu sync.Mutex

	outbound chan rpcEnvelope

	mu        sync.Mutex
	overrides map[string]func(f *fakeAppServer, env rpcEnvelope)
	threadID  string
}

func newFakeAppServer(t *testing.T) *fakeAppServer {
	t.Helper()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	client := newAppServerClient(stdinW, stdoutR, nil)
	f := &fakeAppServer{
		client:    client,
		stdoutW:   stdoutW,
		outbound:  make(chan rpcEnvelope, 64),
		overrides: map[string]func(*fakeAppServer, rpcEnvelope){},
		threadID:  "thread-test-1",
	}
	go f.readLoop(stdinR)
	t.Cleanup(func() {
		client.close()
		_ = stdoutW.Close()
		_ = stdinR.Close()
	})
	return f
}

// install replaces the dialAppServer test seam so codexEngine.Start dials
// this fake instead of exec'ing the real codex binary. Restored on cleanup.
func (f *fakeAppServer) install(t *testing.T) {
	t.Helper()
	orig := dialAppServer
	dialAppServer = func(ctx context.Context, env []string, workDir string) (*appServerClient, error) {
		return f.client, nil
	}
	t.Cleanup(func() { dialAppServer = orig })
}

// readLoop decodes every line the client writes to "stdin", publishes it on
// outbound (non-blocking — most tests never drain it), and auto-responds to
// any call (id+method present) unless a per-method override is registered.
func (f *fakeAppServer) readLoop(stdinR io.Reader) {
	defer close(f.outbound)
	r := bufio.NewReader(stdinR)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var env rpcEnvelope
			if json.Unmarshal(line, &env) == nil {
				select {
				case f.outbound <- env:
				default:
				}
				if env.ID != nil && env.Method != "" {
					f.autoRespond(env)
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (f *fakeAppServer) autoRespond(env rpcEnvelope) {
	f.mu.Lock()
	override := f.overrides[env.Method]
	f.mu.Unlock()
	if override != nil {
		override(f, env)
		return
	}
	switch env.Method {
	case "thread/start":
		f.replyResult(*env.ID, fmt.Sprintf(`{"thread":{"id":%q}}`, f.threadID))
	default:
		f.replyResult(*env.ID, `{}`)
	}
}

// setOverride registers a responder for `method` that replaces the default
// auto-reply, e.g. to script an rpc error or capture request params.
func (f *fakeAppServer) setOverride(method string, fn func(f *fakeAppServer, env rpcEnvelope)) {
	f.mu.Lock()
	f.overrides[method] = fn
	f.mu.Unlock()
}

func (f *fakeAppServer) writeLine(line string) {
	f.writeMu.Lock()
	defer f.writeMu.Unlock()
	_, _ = f.stdoutW.Write([]byte(line + "\n"))
}

// feed injects a raw inbound line (notification or server->client request).
func (f *fakeAppServer) feed(line string) { f.writeLine(line) }

func (f *fakeAppServer) replyResult(id json.RawMessage, resultJSON string) {
	f.writeLine(fmt.Sprintf(`{"id":%s,"result":%s}`, string(id), resultJSON))
}

func (f *fakeAppServer) replyError(id json.RawMessage, code int, msg string) {
	f.writeLine(fmt.Sprintf(`{"id":%s,"error":{"code":%d,"message":%q}}`, string(id), code, msg))
}

// startTestCodexEngine wires a fake app-server, installs it as the dial seam,
// and starts a codexEngine against it with sane defaults for any EngineSpec
// field left zero. Registers Stop as cleanup (idempotent, safe even if the
// test also calls Stop itself).
func startTestCodexEngine(t *testing.T, sink Sink, spec EngineSpec) (*codexEngine, *fakeAppServer) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	f := newFakeAppServer(t)
	f.install(t)
	eng := newCodexEngine(sink)
	if spec.SessionID == "" {
		spec.SessionID = "sess-1"
	}
	if spec.WorkDir == "" {
		spec.WorkDir = t.TempDir()
	}
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)
	return eng, f
}

// collectEventsUntil reads from ch, appending every event, until stop reports
// true, the channel closes, or timeout elapses (fails the test).
func collectEventsUntil(t *testing.T, ch <-chan EngineEvent, stop func(EngineEvent) bool, timeout time.Duration) []EngineEvent {
	t.Helper()
	var got []EngineEvent
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return got
			}
			got = append(got, ev)
			if stop(ev) {
				return got
			}
		case <-deadline:
			t.Fatalf("timed out collecting events, got %d so far: %+v", len(got), got)
		}
	}
}

// waitForEventType returns the first event of the given type, ignoring any
// others in between, or fails the test after timeout.
func waitForEventType(t *testing.T, ch <-chan EngineEvent, want EventType, timeout time.Duration) EngineEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("Events() closed before an event of type %q arrived", want)
			}
			if ev.Type == want {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event type %q", want)
		}
	}
}

// rawFixtureLines returns the non-empty raw JSON lines of a captured
// app-server JSONL fixture, for feeding directly onto a fakeAppServer's wire.
func rawFixtureLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "codex_appserver", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// waitForOutbound returns the first outbound envelope with the given method,
// or fails the test after timeout.
func waitForOutbound(t *testing.T, f *fakeAppServer, method string, timeout time.Duration) rpcEnvelope {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case env, ok := <-f.outbound:
			if !ok {
				t.Fatalf("fake app-server wire closed before %q was sent", method)
			}
			if env.Method == method {
				return env
			}
		case <-deadline:
			t.Fatalf("timed out waiting for outbound %q", method)
		}
	}
}

func countCategory(sink *testSink, category string) int {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	n := 0
	for _, m := range sink.recordedMsgs {
		if m.category == category {
			n++
		}
	}
	return n
}
