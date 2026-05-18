package tools_manifest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/manifest/config"
)

// envCapturingRunner implements python.Runner and records the env slice it receives.
type envCapturingRunner struct {
	mu  sync.Mutex
	env []string
	out []byte
}

func (r *envCapturingRunner) Invoke(_ context.Context, _ string, _ []byte, env []string, _ time.Duration) ([]byte, error) {
	r.mu.Lock()
	r.env = make([]string, len(env))
	copy(r.env, env)
	r.mu.Unlock()
	return r.out, nil
}

func (r *envCapturingRunner) CapturedEnv() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.env))
	copy(out, r.env)
	return out
}

const envAllowManifestYAML = `
tools:
  - name: tool_env_allow
    type: python_script
    description: Tool with env_allow
    script: tools/run.py
    env_allow:
      - "TEST_NRF_PROJ_*"
    input_schema:
      type: object
      properties:
        value:
          type: string
      required: [value]
`

func newEnvAllowManifest(t *testing.T) *config.Manifest {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tool_manifest.yaml"), []byte(envAllowManifestYAML), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	m, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return m
}

func containsEnvEntry(env []string, entry string) bool {
	for _, e := range env {
		if e == entry {
			return true
		}
	}
	return false
}

// TestManifestHandler_ProjectEnv_EnvAllow verifies that project env vars matching
// env_allow patterns reach the python runner, while non-matching vars are excluded.
func TestManifestHandler_ProjectEnv_EnvAllow(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	manifest := newEnvAllowManifest(t)
	runner := &envCapturingRunner{out: []byte(`{"ok":true}`)}
	hub := &fakeHub{}
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)

	projectEnv := []string{"TEST_NRF_PROJ_KEY=myvalue", "UNRELATED_OTHER_VAR=other"}
	prov := New(manifest, runner, projectID, "sess-env-1", dispatchRepo, nil, hub, clk, projectEnv)
	h, ok := prov.Handler("tool_env_allow")
	if !ok {
		t.Fatalf("Handler: tool_env_allow not found")
	}

	_, _, err := h.Invoke(context.Background(), env0, json.RawMessage(`{"value":"x"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	captured := runner.CapturedEnv()
	if !containsEnvEntry(captured, "TEST_NRF_PROJ_KEY=myvalue") {
		t.Errorf("runner env missing TEST_NRF_PROJ_KEY=myvalue (should match env_allow TEST_NRF_PROJ_*); env=%v", captured)
	}
	if containsEnvEntry(captured, "UNRELATED_OTHER_VAR=other") {
		t.Errorf("runner env unexpectedly contains UNRELATED_OTHER_VAR=other (blocked by env_allow TEST_NRF_PROJ_*); env=%v", captured)
	}
}

// TestManifestHandler_ProjectEnv_NoAllowMatch verifies that project env vars not
// matching any env_allow pattern are excluded from the runner's environment.
func TestManifestHandler_ProjectEnv_NoAllowMatch(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	manifest := newEnvAllowManifest(t)
	runner := &envCapturingRunner{out: []byte(`{"ok":true}`)}
	hub := &fakeHub{}
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)

	projectEnv := []string{"COMPLETELY_DIFFERENT_VAR=blocked"}
	prov := New(manifest, runner, projectID, "sess-env-2", dispatchRepo, nil, hub, clk, projectEnv)
	h, ok := prov.Handler("tool_env_allow")
	if !ok {
		t.Fatalf("Handler: tool_env_allow not found")
	}

	_, _, err := h.Invoke(context.Background(), env0, json.RawMessage(`{"value":"x"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if containsEnvEntry(runner.CapturedEnv(), "COMPLETELY_DIFFERENT_VAR=blocked") {
		t.Errorf("runner env should not contain COMPLETELY_DIFFERENT_VAR=blocked (no match for TEST_NRF_PROJ_*)")
	}
}

// TestManifestHandler_ProjectEnv_LastWinsOrder verifies that when the same key
// exists in os.Environ and in projectEnv, the projectEnv entry is the last occurrence
// in candidates — confirming projectEnv trails os.Environ so exec.Cmd last-wins
// semantics favour the project-supplied value.
// Serial (no t.Parallel) because it uses t.Setenv.
func TestManifestHandler_ProjectEnv_LastWinsOrder(t *testing.T) {
	const testKey = "TEST_NRF_PROJ_ORDER_KEY"
	t.Setenv(testKey, "os_value")

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	manifest := newEnvAllowManifest(t)
	runner := &envCapturingRunner{out: []byte(`{"ok":true}`)}
	hub := &fakeHub{}
	dispatchRepo, projectID := newTestDispatchRepo(t, clk)

	projectEnv := []string{testKey + "=proj_value"}
	prov := New(manifest, runner, projectID, "sess-env-3", dispatchRepo, nil, hub, clk, projectEnv)
	h, ok := prov.Handler("tool_env_allow")
	if !ok {
		t.Fatalf("Handler: tool_env_allow not found")
	}

	_, _, err := h.Invoke(context.Background(), env0, json.RawMessage(`{"value":"x"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	captured := runner.CapturedEnv()
	if !containsEnvEntry(captured, testKey+"=proj_value") {
		t.Errorf("runner env missing %s=proj_value; env=%v", testKey, captured)
	}
	// proj_value must be the last occurrence of testKey, confirming projectEnv comes after os.Environ.
	lastVal := ""
	for _, e := range captured {
		if strings.HasPrefix(e, testKey+"=") {
			lastVal = strings.TrimPrefix(e, testKey+"=")
		}
	}
	if lastVal != "proj_value" {
		t.Errorf("last value for %s=%q, want proj_value (projectEnv must trail os.Environ in candidates)", testKey, lastVal)
	}
}
