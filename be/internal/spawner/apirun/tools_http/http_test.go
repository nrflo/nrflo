package tools_http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/spawner/apirun"
)

func toolDef(name, endpoint, authMethod string, authRef *string, timeout int) *model.ToolDefinition {
	return &model.ToolDefinition{
		Name:        name,
		Description: "test tool",
		Endpoint:    endpoint,
		AuthMethod:  authMethod,
		AuthRef:     authRef,
		TimeoutSec:  timeout,
	}
}

func sampleEnv() apirun.ToolEnv {
	return apirun.ToolEnv{
		ProjectID:    "p1",
		WorkflowName: "wf",
		SessionID:    "sess-1",
		Clock:        clock.Real(),
	}
}

func TestHTTPTool_BodyShapeAndSuccess(t *testing.T) {
	var captured map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	h := New(nil)(toolDef("lookup", srv.URL, "none", nil, 5))
	out, isErr, err := h.Invoke(context.Background(), sampleEnv(), json.RawMessage(`{"sku":"X"}`))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true output=%q", out)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Errorf("output=%q want ok:true", out)
	}
	if captured["tool"] != "lookup" {
		t.Errorf("body.tool=%v want lookup", captured["tool"])
	}
	ctx, ok := captured["context"].(map[string]interface{})
	if !ok || ctx["project_id"] != "p1" || ctx["workflow"] != "wf" || ctx["session_id"] != "sess-1" {
		t.Errorf("body.context=%v want project=p1 workflow=wf session=sess-1", captured["context"])
	}
	input, ok := captured["input"].(map[string]interface{})
	if !ok || input["sku"] != "X" {
		t.Errorf("body.input=%v want sku=X", captured["input"])
	}
}

func TestHTTPTool_BearerEnv(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("MY_TOKEN", "secret-123")
	ref := "MY_TOKEN"
	h := New(nil)(toolDef("t", srv.URL, "bearer_env", &ref, 5))
	if _, isErr, err := h.Invoke(context.Background(), sampleEnv(), json.RawMessage(`{}`)); err != nil || isErr {
		t.Fatalf("Invoke: err=%v isErr=%v", err, isErr)
	}
	if got != "Bearer secret-123" {
		t.Errorf("Authorization=%q, want Bearer secret-123", got)
	}
}

func TestHTTPTool_BearerSecretRef_Env(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("SR_TOKEN", "from-env")
	ref := "env:SR_TOKEN"
	h := New(nil)(toolDef("t", srv.URL, "bearer_secret_ref", &ref, 5))
	if _, _, err := h.Invoke(context.Background(), sampleEnv(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got != "Bearer from-env" {
		t.Errorf("Authorization=%q, want Bearer from-env", got)
	}
}

func TestHTTPTool_BearerSecretRef_File(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	path := filepath.Join(dir, "tok.txt")
	if err := os.WriteFile(path, []byte("from-file\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	ref := "file:" + path
	h := New(nil)(toolDef("t", srv.URL, "bearer_secret_ref", &ref, 5))
	if _, _, err := h.Invoke(context.Background(), sampleEnv(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got != "Bearer from-file" {
		t.Errorf("Authorization=%q, want Bearer from-file", got)
	}
}

func TestHTTPTool_BearerSecretRef_Literal(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(srv.Close)

	ref := "literal:LITERALVAL"
	h := New(nil)(toolDef("t", srv.URL, "bearer_secret_ref", &ref, 5))
	if _, _, err := h.Invoke(context.Background(), sampleEnv(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got != "Bearer LITERALVAL" {
		t.Errorf("Authorization=%q, want Bearer LITERALVAL", got)
	}
}

func TestHTTPTool_4xx_NoRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`bad`))
	}))
	t.Cleanup(srv.Close)

	h := New(nil)(toolDef("t", srv.URL, "none", nil, 5))
	out, isErr, err := h.Invoke(context.Background(), sampleEnv(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true on 4xx")
	}
	if !strings.Contains(out, "bad") {
		t.Errorf("output=%q want contains 'bad'", out)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls=%d want 1 (no retry on 4xx)", got)
	}
}

func TestHTTPTool_5xx_RetryOnce(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`oops`))
			return
		}
		_, _ = w.Write([]byte(`ok-2`))
	}))
	t.Cleanup(srv.Close)

	h := New(nil)(toolDef("t", srv.URL, "none", nil, 5))
	start := time.Now()
	out, isErr, err := h.Invoke(context.Background(), sampleEnv(), json.RawMessage(`{}`))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if isErr || out != "ok-2" {
		t.Errorf("isErr=%v out=%q want ok-2 / false", isErr, out)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls=%d want 2 (retry once)", got)
	}
	if elapsed < 400*time.Millisecond {
		t.Errorf("elapsed=%v want >=400ms (500ms backoff)", elapsed)
	}
}

func TestHTTPTool_5xx_RetryStillFails(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`always-bad`))
	}))
	t.Cleanup(srv.Close)

	h := New(nil)(toolDef("t", srv.URL, "none", nil, 5))
	out, isErr, err := h.Invoke(context.Background(), sampleEnv(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false want true on persistent 5xx")
	}
	if !strings.Contains(out, "always-bad") {
		t.Errorf("output=%q want contains always-bad", out)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls=%d want 2 (one retry then give up)", got)
	}
}

func TestHTTPTool_Timeout(t *testing.T) {
	// Bound the handler so srv.Close doesn't deadlock if the request context
	// stays open after the client times out.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)

	h := New(nil)(toolDef("t", srv.URL, "none", nil, 1))
	out, isErr, err := h.Invoke(context.Background(), sampleEnv(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false want true on timeout")
	}
	if !strings.Contains(strings.ToLower(out), "deadline") &&
		!strings.Contains(strings.ToLower(out), "context") &&
		!strings.Contains(strings.ToLower(out), "timeout") {
		t.Errorf("output=%q want timeout/deadline error", out)
	}
}

func TestHTTPTool_TruncatesOver16KB(t *testing.T) {
	big := strings.Repeat("a", maxResponseBytes+5000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(big))
	}))
	t.Cleanup(srv.Close)

	h := New(nil)(toolDef("t", srv.URL, "none", nil, 5))
	out, isErr, err := h.Invoke(context.Background(), sampleEnv(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if isErr {
		t.Fatalf("isErr=true output=%q", out[:80])
	}
	if !strings.HasSuffix(out, truncSuffix) {
		t.Errorf("output missing truncation suffix; ends with %q", out[len(out)-30:])
	}
	if len(out) != maxResponseBytes+len(truncSuffix) {
		t.Errorf("len(out)=%d want %d", len(out), maxResponseBytes+len(truncSuffix))
	}
}

func TestHTTPTool_BearerEnv_MissingEnv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`unreachable`))
	}))
	t.Cleanup(srv.Close)

	ref := "MISSING_VAR_X"
	os.Unsetenv("MISSING_VAR_X")
	h := New(nil)(toolDef("t", srv.URL, "bearer_env", &ref, 5))
	out, isErr, err := h.Invoke(context.Background(), sampleEnv(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false want true for missing env")
	}
	if !strings.Contains(out, "auth:") {
		t.Errorf("output=%q want auth error", out)
	}
}
