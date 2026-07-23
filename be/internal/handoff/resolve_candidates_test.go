package handoff

import (
	"path/filepath"
	"testing"
)

// TestResolvePathCandidates_ThreeWayStatus exercises the PathResolved /
// PathAmbiguous / PathUnresolved outcomes directly, verifying resolvePaths
// (the collapsing wrapper) still partitions the same way at the higher
// level — the resolvePaths tests in resolve_test.go must not regress.
func TestResolvePathCandidates_ThreeWayStatus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mkFile(t, root, "src/components/Widget.tsx")
	mkFile(t, root, "a/dup.go")
	mkFile(t, root, "b/dup.go")

	results := ResolvePathCandidates(root, []string{"Widget.tsx", "dup.go", "missing.go"})
	if len(results) != 3 {
		t.Fatalf("results = %v, want 3 entries", results)
	}

	resolved := results[0]
	if resolved.Status != PathResolved {
		t.Errorf("Widget.tsx status = %v, want PathResolved", resolved.Status)
	}
	if resolved.Candidate != "Widget.tsx" {
		t.Errorf("Widget.tsx Candidate = %q, want verbatim input", resolved.Candidate)
	}
	if resolved.Resolved != filepath.Join("src", "components", "Widget.tsx") {
		t.Errorf("Widget.tsx Resolved = %q, want src/components/Widget.tsx", resolved.Resolved)
	}

	ambiguous := results[1]
	if ambiguous.Status != PathAmbiguous {
		t.Errorf("dup.go status = %v, want PathAmbiguous", ambiguous.Status)
	}
	if ambiguous.Resolved != "" {
		t.Errorf("dup.go Resolved = %q, want empty (ambiguous never picks one)", ambiguous.Resolved)
	}

	unresolved := results[2]
	if unresolved.Status != PathUnresolved {
		t.Errorf("missing.go status = %v, want PathUnresolved", unresolved.Status)
	}
	if unresolved.Resolved != "" {
		t.Errorf("missing.go Resolved = %q, want empty", unresolved.Resolved)
	}
}

// TestResolvePathCandidates_SlashCandidatesNeverAmbiguous verifies absolute
// and slash-containing candidates only ever land Resolved or Unresolved
// (they Stat directly, never consult the basename index).
func TestResolvePathCandidates_SlashCandidatesNeverAmbiguous(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mkFile(t, root, "pkg/foo.go")

	results := ResolvePathCandidates(root, []string{"pkg/foo.go", "pkg/missing.go"})
	for _, r := range results {
		if r.Status == PathAmbiguous {
			t.Errorf("slash-containing candidate %q landed PathAmbiguous, want Resolved or Unresolved only", r.Candidate)
		}
	}
	if results[0].Status != PathResolved {
		t.Errorf("pkg/foo.go status = %v, want PathResolved", results[0].Status)
	}
	if results[1].Status != PathUnresolved {
		t.Errorf("pkg/missing.go status = %v, want PathUnresolved", results[1].Status)
	}
}

// TestResolvePathCandidates_EmptyRootUnresolvesAll verifies an empty root
// marks every candidate PathUnresolved without panicking.
func TestResolvePathCandidates_EmptyRootUnresolvesAll(t *testing.T) {
	t.Parallel()
	results := ResolvePathCandidates("", []string{"a.go", "b.go"})
	for _, r := range results {
		if r.Status != PathUnresolved {
			t.Errorf("candidate %q status = %v, want PathUnresolved for empty root", r.Candidate, r.Status)
		}
	}
}

// TestResolvePathCandidates_NoCandidatesReturnsNil verifies the empty-input
// short circuit.
func TestResolvePathCandidates_NoCandidatesReturnsNil(t *testing.T) {
	t.Parallel()
	if got := ResolvePathCandidates(t.TempDir(), nil); got != nil {
		t.Errorf("ResolvePathCandidates(no candidates) = %v, want nil", got)
	}
}
