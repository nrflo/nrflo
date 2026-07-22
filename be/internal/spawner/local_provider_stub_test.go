package spawner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// localProviderStub is an in-process OpenAI-compatible Chat Completions SSE
// server standing in for a real local Ollama endpoint: it is the sole
// destination custom.New(Wire=chat_completions)->openaichat's real HTTP
// client talks to, so a test wired up against it exercises the genuine wire
// frame (not a fake RoundTripper) with no cloud provider ever contacted.
// Each incoming request is served the next configured script in order — one
// script per turn — mirroring mock.Provider's script-per-Run()-call shape
// used by delegateWorkerScripts, but over real HTTP.
type localProviderStub struct {
	srv *httptest.Server

	mu       sync.Mutex
	scripts  []string
	next     int
	authSeen []string

	requests int32
}

// newLocalProviderStub starts the stub server and registers it for cleanup.
// scripts[i] is the raw SSE response body served for the (i+1)-th request;
// a request beyond len(scripts) fails the test immediately rather than
// hanging the runner loop.
func newLocalProviderStub(t *testing.T, scripts []string) *localProviderStub {
	t.Helper()
	stub := &localProviderStub{scripts: scripts}
	stub.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&stub.requests, 1)

		stub.mu.Lock()
		idx := stub.next
		stub.next++
		stub.authSeen = append(stub.authSeen, r.Header.Get("Authorization"))
		stub.mu.Unlock()

		if idx >= len(stub.scripts) {
			t.Errorf("localProviderStub: unexpected request #%d (only %d scripts configured)", idx+1, len(stub.scripts))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := w.Write([]byte(stub.scripts[idx])); err != nil {
			t.Errorf("localProviderStub: write response: %v", err)
		}
	}))
	t.Cleanup(stub.srv.Close)
	return stub
}

func (s *localProviderStub) URL() string { return s.srv.URL }

func (s *localProviderStub) requestCount() int { return int(atomic.LoadInt32(&s.requests)) }

// authHeaders returns every Authorization header value seen, in request
// order — used to assert the stub was hit with no bearer token (local
// providers with a blank api_key never send one, per openaichat.New).
func (s *localProviderStub) authHeaders() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.authSeen))
	copy(out, s.authSeen)
	return out
}

// jsonString marshals a Go string to its JSON-quoted representation, used to
// build inline chunk JSON below.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// sseChunk builds one Chat Completions streaming frame carrying deltaJSON in
// choices[0].delta, terminated per the SSE data-frame contract (data: ...\n\n)
// — mirrors chunkJSON in openaichat/decode_test.go, the wire-frame reference.
func sseChunk(deltaJSON, finishReason string) string {
	fr := "null"
	if finishReason != "" {
		fr = jsonString(finishReason)
	}
	return "data: " + fmt.Sprintf(
		`{"id":"chatcmpl-stub","object":"chat.completion.chunk","created":1,"model":"stub","choices":[{"index":0,"delta":%s,"finish_reason":%s}]}`,
		deltaJSON, fr,
	) + "\n\n"
}

func sseDone() string { return "data: [DONE]\n\n" }

// sseToolCallScript builds a full single-frame tool-call turn: one chunk
// carrying the tool's id/name/arguments (Chat Completions accepts the full
// arguments string in one delta, per decode.go's per-index accumulation)
// with finish_reason="tool_calls", followed by [DONE].
func sseToolCallScript(toolID, toolName string, args json.RawMessage) string {
	delta := fmt.Sprintf(
		`{"tool_calls":[{"index":0,"id":%s,"function":{"name":%s,"arguments":%s}}]}`,
		jsonString(toolID), jsonString(toolName), jsonString(string(args)),
	)
	return sseChunk(delta, "tool_calls") + sseDone()
}

// sseTextScript builds a plain-text final turn (finish_reason="stop", mapped
// to stop_reason "end_turn" by mapFinishReason) so the runner loop's
// end_turn branch fires PASS.
func sseTextScript(text string) string {
	delta := fmt.Sprintf(`{"content":%s}`, jsonString(text))
	return sseChunk(delta, "stop") + sseDone()
}
