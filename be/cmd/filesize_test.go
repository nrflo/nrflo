package cmd

// Tests for scripts/filesize.sh — the repo-wide source-file line-count ratchet
// gate wired into `make filesize`. The gate has no Go surface, so these tests
// drive the real shell script against hermetic temp git repos (matching the
// TestMakefileTargets_* / binaries_test.go pattern that exercises repo tooling
// via os/exec). LIMIT is 300; over-limit tracked .go/.ts/.tsx files are
// snapshotted into filesize.baseline and may only shrink.

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// filesizeScriptPath returns the absolute path to scripts/filesize.sh.
func filesizeScriptPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join(filepath.Dir(getBeDir(t)), "scripts", "filesize.sh")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("filesize.sh not found at %s: %v", p, err)
	}
	return p
}

// gitCmd runs a git command in dir, failing the test on error.
func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// initRepo creates an isolated git repo under t.TempDir().
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-q")
	return dir
}

// addAll stages the entire worktree (including deletions) into the index so
// git ls-files reflects the current state. No commit is needed.
func addAll(t *testing.T, repo string) {
	t.Helper()
	gitCmd(t, repo, "add", "-A")
}

// writeLines writes rel inside repo with exactly n newline-terminated lines, so
// `wc -l` reports n.
func writeLines(t *testing.T, repo, rel string, n int) {
	t.Helper()
	full := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("// line\n")
	}
	if err := os.WriteFile(full, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// rmFile deletes rel from the worktree.
func rmFile(t *testing.T, repo, rel string) {
	t.Helper()
	if err := os.Remove(filepath.Join(repo, rel)); err != nil {
		t.Fatalf("remove %s: %v", rel, err)
	}
}

// runFilesize runs `sh <script> <mode>` with cwd=repo and returns the exit
// code, stdout, and stderr. A non-ExitError (e.g. sh missing) fails the test.
func runFilesize(t *testing.T, repo, script, mode string) (int, string, string) {
	t.Helper()
	cmd := exec.Command("sh", script, mode)
	cmd.Dir = repo
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("filesize.sh %s: %v (stderr=%s)", mode, err, errb.String())
		}
		return ee.ExitCode(), out.String(), errb.String()
	}
	return 0, out.String(), errb.String()
}

// TestFilesizeClassification exercises the four check classifications plus
// boundary and extension-filter behavior. Each case snapshots a baseline state
// via `update`, mutates the tree, then asserts `check`'s exit code + output.
func TestFilesizeClassification(t *testing.T) {
	script := filesizeScriptPath(t)
	cases := []struct {
		name     string
		setup    func(t *testing.T, repo string) // baseline-state tree
		mutate   func(t *testing.T, repo string) // change before check
		wantExit int
		wantOut  []string
	}{
		{
			name:     "clean_tree_passes",
			setup:    func(t *testing.T, r string) { writeLines(t, r, "be/big.go", 305) },
			mutate:   func(t *testing.T, r string) {},
			wantExit: 0,
			wantOut:  []string{"no new files over"},
		},
		{
			// anchor.go is baselined and stays put, so the only offender is the
			// newly added file.
			name:     "new_file_over_limit",
			setup:    func(t *testing.T, r string) { writeLines(t, r, "anchor.go", 305) },
			mutate:   func(t *testing.T, r string) { writeLines(t, r, "ui/new.tsx", 301) },
			wantExit: 1,
			wantOut:  []string{"NEW", "ui/new.tsx", "301"},
		},
		{
			name:     "regrowth_beyond_baseline",
			setup:    func(t *testing.T, r string) { writeLines(t, r, "a.go", 305) },
			mutate:   func(t *testing.T, r string) { writeLines(t, r, "a.go", 312) },
			wantExit: 1,
			wantOut:  []string{"REGROWTH", "a.go", "312", "305"},
		},
		{
			name:     "stale_shrunk_below_limit",
			setup:    func(t *testing.T, r string) { writeLines(t, r, "a.go", 340) },
			mutate:   func(t *testing.T, r string) { writeLines(t, r, "a.go", 250) },
			wantExit: 1,
			wantOut:  []string{"STALE", "a.go"},
		},
		{
			name: "stale_deleted_file",
			setup: func(t *testing.T, r string) {
				writeLines(t, r, "a.go", 340)
				writeLines(t, r, "keep.go", 10)
			},
			mutate:   func(t *testing.T, r string) { rmFile(t, r, "a.go") },
			wantExit: 1,
			wantOut:  []string{"STALE", "a.go"},
		},
		{
			name:     "accepted_shrink_still_over_limit",
			setup:    func(t *testing.T, r string) { writeLines(t, r, "a.go", 360) },
			mutate:   func(t *testing.T, r string) { writeLines(t, r, "a.go", 320) },
			wantExit: 0,
			wantOut:  []string{"no new files over"},
		},
		{
			name:     "boundary_exactly_300_not_flagged",
			setup:    func(t *testing.T, r string) { writeLines(t, r, "anchor.go", 305) },
			mutate:   func(t *testing.T, r string) { writeLines(t, r, "edge.go", 300) },
			wantExit: 0,
			wantOut:  []string{"no new files over"},
		},
		{
			name:  "non_source_extensions_ignored",
			setup: func(t *testing.T, r string) { writeLines(t, r, "anchor.go", 305) },
			mutate: func(t *testing.T, r string) {
				writeLines(t, r, "docs/big.md", 500)
				writeLines(t, r, "data.py", 500)
			},
			wantExit: 0,
			wantOut:  []string{"no new files over"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initRepo(t)
			tc.setup(t, repo)
			addAll(t, repo)
			if code, _, errb := runFilesize(t, repo, script, "update"); code != 0 {
				t.Fatalf("update exit=%d, want 0 (stderr=%s)", code, errb)
			}
			tc.mutate(t, repo)
			addAll(t, repo)
			code, out, _ := runFilesize(t, repo, script, "check")
			if code != tc.wantExit {
				t.Errorf("check exit=%d, want %d\noutput:\n%s", code, tc.wantExit, out)
			}
			for _, sub := range tc.wantOut {
				if !strings.Contains(out, sub) {
					t.Errorf("check output missing %q\noutput:\n%s", sub, out)
				}
			}
		})
	}
}

// TestFilesizeUpdateResolvesViolation verifies update is the documented
// remediation: a NEW violation becomes green after re-snapshotting, and the
// regenerated baseline carries the ratchet header + the offending path.
func TestFilesizeUpdateResolvesViolation(t *testing.T) {
	script := filesizeScriptPath(t)
	repo := initRepo(t)
	writeLines(t, repo, "anchor.go", 305) // a pre-existing baselined over-limit file
	addAll(t, repo)
	if code, _, errb := runFilesize(t, repo, script, "update"); code != 0 {
		t.Fatalf("initial update exit=%d (stderr=%s)", code, errb)
	}

	writeLines(t, repo, "fresh.go", 305)
	addAll(t, repo)
	if code, out, _ := runFilesize(t, repo, script, "check"); code != 1 {
		t.Fatalf("check after adding over-limit file: exit=%d, want 1\n%s", code, out)
	}

	if code, _, errb := runFilesize(t, repo, script, "update"); code != 0 {
		t.Fatalf("re-snapshot update exit=%d (stderr=%s)", code, errb)
	}
	if code, out, _ := runFilesize(t, repo, script, "check"); code != 0 {
		t.Fatalf("check after re-snapshot: exit=%d, want 0\n%s", code, out)
	}

	data, err := os.ReadFile(filepath.Join(repo, "filesize.baseline"))
	if err != nil {
		t.Fatalf("read regenerated baseline: %v", err)
	}
	body := string(data)
	if !strings.HasPrefix(body, "# filesize.baseline") {
		t.Errorf("baseline missing ratchet header, got first line:\n%s", strings.SplitN(body, "\n", 2)[0])
	}
	if !strings.Contains(body, "fresh.go\t305") {
		t.Errorf("baseline missing \"fresh.go\\t305\" entry:\n%s", body)
	}
}

// TestFilesizeUpdateZeroOverLimit guards the edge case where no tracked source
// file exceeds the limit: `update` must still exit 0 and write a header-only
// baseline, and a subsequent `check` against it must pass. This is the end state
// once every oversized file is split, so `make filesize-update` must not choke
// on it (grep -c exits 1 on zero matches; the script guards that with `|| true`).
func TestFilesizeUpdateZeroOverLimit(t *testing.T) {
	script := filesizeScriptPath(t)
	repo := initRepo(t)
	writeLines(t, repo, "small.go", 10) // under the limit -> empty snapshot
	addAll(t, repo)

	code, _, errb := runFilesize(t, repo, script, "update")
	if code != 0 {
		t.Errorf("update with zero over-limit files exit=%d, want 0 (stderr=%s)", code, errb)
	}
	data, err := os.ReadFile(filepath.Join(repo, "filesize.baseline"))
	if err != nil {
		t.Fatalf("baseline not written: %v", err)
	}
	if !strings.HasPrefix(string(data), "# filesize.baseline") {
		t.Errorf("baseline missing ratchet header")
	}
	// A subsequent check works against the header-only baseline.
	if code, out, _ := runFilesize(t, repo, script, "check"); code != 0 {
		t.Errorf("check against empty baseline exit=%d, want 0\n%s", code, out)
	}
}

// TestFilesizeUsageError verifies an unknown mode exits 2 with a usage message.
func TestFilesizeUsageError(t *testing.T) {
	script := filesizeScriptPath(t)
	repo := initRepo(t)
	code, _, errb := runFilesize(t, repo, script, "bogus")
	if code != 2 {
		t.Errorf("unknown mode exit=%d, want 2", code)
	}
	if !strings.Contains(errb, "usage") {
		t.Errorf("unknown mode stderr missing %q, got: %s", "usage", errb)
	}
}

// TestFilesizeBaselineMatchesHEAD guards that the committed filesize.baseline is
// in sync with the working tree — i.e. `make filesize` is green, as the ratchet
// requires. A failure means a tracked source file crossed/regrew past 300 lines
// (or a baselined file dropped) without `make filesize-update`.
func TestFilesizeBaselineMatchesHEAD(t *testing.T) {
	script := filesizeScriptPath(t)
	repoRoot := filepath.Dir(getBeDir(t))
	code, out, errb := runFilesize(t, repoRoot, script, "check")
	if code != 0 {
		t.Errorf("`make filesize` not green on the working tree (exit=%d).\n"+
			"Run `make filesize-update` to re-snapshot filesize.baseline.\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
}
