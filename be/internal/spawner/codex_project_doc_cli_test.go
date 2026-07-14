//go:build clitools

package spawner

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This file spawns the real `codex` binary and is excluded from `make test`
// (no build tags) by design — CLAUDE.md rule 4 forbids real CLI execution in
// the default suite. Run deliberately after a codex version bump:
//
//	go test -tags clitools ./internal/spawner/ -run CodexProjectDoc -v
//
// It is the drift alarm for codexProjectDocArgs(): a renamed/removed codex
// key would otherwise make the -c override a silent no-op instead of failing
// loudly. `codex debug prompt-input` renders the model-visible prompt as JSON
// with no API call, no auth, sub-second.
const (
	markerRoot = "MARKER_ROOT_9f3a"
	markerPkg  = "MARKER_PKG_9f3a"
)

// codexProjectDocFixture builds a git repo with a root CLAUDE.md (deliberately
// no AGENTS.md — this is the case the fix repairs) and a nested sub/pkg/CLAUDE.md,
// returning the fixture root.
func codexProjectDocFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", root, err, out)
	}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(markerRoot), 0o644); err != nil {
		t.Fatalf("write root CLAUDE.md: %v", err)
	}
	pkgDir := filepath.Join(root, "sub", "pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir sub/pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "CLAUDE.md"), []byte(markerPkg), 0o644); err != nil {
		t.Fatalf("write sub/pkg/CLAUDE.md: %v", err)
	}
	return root
}

// runCodexPromptInput runs `codex debug prompt-input` with the given extra
// args from cwd, returning combined stdout+stderr. codexHome is an isolated,
// empty CODEX_HOME so the developer's real ~/.codex config cannot influence
// the result.
func runCodexPromptInput(t *testing.T, cwd, codexHome string, extraArgs ...string) string {
	t.Helper()
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex binary not found in PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := append([]string{"debug", "prompt-input"}, extraArgs...)
	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "CODEX_HOME="+codexHome)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("codex %v (dir=%s): %v\noutput:\n%s", args, cwd, err, out.String())
	}
	return out.String()
}

// TestCodexProjectDocArgs_LoadsRootDocAtSpawnCwd is the end-to-end drift alarm
// for the PRODUCTION spawn configuration: every nrflo codex spawn runs with
// cwd = Config.ProjectRoot (spawner_prepare.go, spawner_script.go,
// spawn_observer.go -> codex_appserver_client.go `cmd.Dir = workDir`), so the
// root->cwd project-doc chain codex walks is always exactly one directory: the
// repo root.
//
// It therefore asserts what the flag actually buys (the root CLAUDE.md is
// loaded in a repo with no AGENTS.md) AND what it deliberately does not (a
// nested package CLAUDE.md is NOT loaded). The negative assertion is the
// important one: codex only walks ANCESTORS of cwd, never descendants, so
// nested package docs cannot reach a codex worker by this mechanism. If a
// future codex version starts loading descendants, this test fails loudly and
// the limitation documented in REFERENCE.md must be revisited.
func TestCodexProjectDocArgs_LoadsRootDocAtSpawnCwd(t *testing.T) {
	root := codexProjectDocFixture(t)
	codexHome := t.TempDir()

	baseline := runCodexPromptInput(t, root, codexHome)
	if strings.Contains(baseline, markerRoot) {
		t.Fatalf("baseline (no flags) unexpectedly contains %s — the gap codexProjectDocArgs() fixes is not reproducible with this codex version; drift alarm is vacuous:\n%s", markerRoot, baseline)
	}

	withFlag := runCodexPromptInput(t, root, codexHome, codexProjectDocArgs()...)
	if !strings.Contains(withFlag, markerRoot) {
		t.Errorf("with codexProjectDocArgs() (cwd=root, the production spawn cwd), output missing %s — the -c override no longer loads the root project doc:\n%s", markerRoot, withFlag)
	}
	if strings.Contains(withFlag, markerPkg) {
		t.Errorf("with codexProjectDocArgs() (cwd=root), output unexpectedly contains %s — codex now loads DESCENDANT project docs. This is strictly better than the documented behavior; update REFERENCE.md (which states nested package docs never reach codex workers) and this assertion:\n%s", markerPkg, withFlag)
	}
}

// TestCodexProjectDocArgs_NestedDocsNeedNestedCwd documents the mechanism
// behind the limitation above: the fallback names DO apply at every level of
// the chain, but the chain is rooted at cwd and walks upward. From cwd=sub/pkg
// both docs load; from cwd=root (what nrflo actually does) only the root does.
// This test exists to prove the negative assertion above is a property of
// codex's cwd-anchored chain, not of a broken flag value.
func TestCodexProjectDocArgs_NestedDocsNeedNestedCwd(t *testing.T) {
	root := codexProjectDocFixture(t)
	codexHome := t.TempDir()
	pkgDir := filepath.Join(root, "sub", "pkg")

	out := runCodexPromptInput(t, pkgDir, codexHome, codexProjectDocArgs()...)
	if !strings.Contains(out, markerRoot) {
		t.Errorf("cwd=sub/pkg: output missing %s — the root->cwd chain is not being walked:\n%s", markerRoot, out)
	}
	if !strings.Contains(out, markerPkg) {
		t.Errorf("cwd=sub/pkg: output missing %s — the fallback filenames are not applied at the cwd level:\n%s", markerPkg, out)
	}
}
