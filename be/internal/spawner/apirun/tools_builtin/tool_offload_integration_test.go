package tools_builtin

// End-to-end tests for apirun.MaybeOffloadToolResult against the real
// artifact service + DB harness (the pure guard-path tests live in apirun).

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
)

func TestToolResultOffload_EndToEnd(t *testing.T) {
	e := newBuiltinTestEnv(t)
	big := strings.Repeat("A", 9000) + strings.Repeat("Z", 9000)

	out := apirun.MaybeOffloadToolResult(context.Background(), e.env, "bash", big)
	if out == big {
		t.Fatal("large result was not offloaded")
	}
	if !strings.Contains(out, "artifact") || !strings.Contains(out, "toolres_bash_") {
		t.Fatalf("excerpt missing artifact pointer: %q", out[:200])
	}
	if !strings.HasPrefix(out, "AAAA") || !strings.HasSuffix(out, "ZZZZ") {
		t.Error("excerpt should keep head and tail of the original output")
	}
	if len(out) >= len(big)/2 {
		t.Errorf("excerpt not bounded: %d bytes of %d", len(out), len(big))
	}

	// Artifact row exists with the full payload size.
	arts, err := e.env.ArtifactSvc.List(context.Background(), testWFIID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	var found bool
	for _, a := range arts {
		if strings.HasPrefix(a.Name, "toolres_bash_") {
			found = true
			if a.SizeBytes != int64(len(big)) {
				t.Errorf("artifact size = %d, want %d", a.SizeBytes, len(big))
			}
		}
	}
	if !found {
		t.Fatal("no toolres artifact row created")
	}

	// Identical content again: content-addressed name collides — must still
	// return a pointer excerpt (existing artifact holds the same bytes), and
	// must not create a second row.
	out2 := apirun.MaybeOffloadToolResult(context.Background(), e.env, "bash", big)
	if !strings.Contains(out2, "toolres_bash_") {
		t.Errorf("repeat offload lost the artifact pointer: %q", out2[:200])
	}
	arts2, _ := e.env.ArtifactSvc.List(context.Background(), testWFIID)
	if len(arts2) != len(arts) {
		t.Errorf("duplicate artifact rows created: %d -> %d", len(arts), len(arts2))
	}
}

// TestToolResultOffload_ReadFileEndToEnd verifies the native FS bridge's
// read_file result quarantines through the same offload path as any other
// tool: a >8KB file read gets stored as a toolres_read_file_ artifact and
// replaced by a bounded excerpt.
func TestToolResultOffload_ReadFileEndToEnd(t *testing.T) {
	e := newBuiltinTestEnv(t)
	e.env.WorkDir = t.TempDir()

	big := strings.Repeat("A", 9000)
	if err := os.WriteFile(filepath.Join(e.env.WorkDir, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatalf("write big.txt: %v", err)
	}

	h := FSTools()["read_file"]
	out, isErr, err := h.Invoke(context.Background(), e.env, json.RawMessage(`{"path":"big.txt"}`))
	if err != nil || isErr {
		t.Fatalf("read_file = (%q, %v, %v)", out, isErr, err)
	}

	offloaded := apirun.MaybeOffloadToolResult(context.Background(), e.env, "read_file", out)
	if offloaded == out {
		t.Fatal("large read_file result was not offloaded")
	}
	if !strings.Contains(offloaded, "toolres_read_file_") {
		t.Fatalf("excerpt missing toolres_read_file_ artifact pointer: %q", offloaded[:200])
	}

	arts, err := e.env.ArtifactSvc.List(context.Background(), testWFIID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	var found bool
	for _, a := range arts {
		if strings.HasPrefix(a.Name, "toolres_read_file_") {
			found = true
		}
	}
	if !found {
		t.Fatal("no toolres_read_file_ artifact row created")
	}
}

func TestToolResultOffload_ConfigDisable(t *testing.T) {
	e := newBuiltinTestEnv(t)
	mustExec(t, e.pool, `INSERT INTO config (project_id, key, value) VALUES ('', 'tool_result_offload_enabled', 'false')`)

	big := strings.Repeat("B", 50_000)
	out := apirun.MaybeOffloadToolResult(context.Background(), e.env, "bash", big)
	if out != big {
		t.Error("offload disabled by config must pass through unchanged")
	}
}

func TestToolResultOffload_ThresholdOverride(t *testing.T) {
	e := newBuiltinTestEnv(t)
	mustExec(t, e.pool, `INSERT INTO config (project_id, key, value) VALUES ('', 'tool_result_offload_threshold_bytes', '100000')`)

	mid := strings.Repeat("C", 20_000) // over default 8KB, under override
	out := apirun.MaybeOffloadToolResult(context.Background(), e.env, "bash", mid)
	if out != mid {
		t.Error("result under configured threshold must pass through")
	}
}
