package spawner

import (
	"context"
	"testing"
	"time"
)

// fakeRefinerySidecar records FoldNow calls and can write the slot digest the
// forced fold would have produced, so a test can distinguish "asked the
// refinery to fold" from "found a digest that already existed".
type fakeRefinerySidecar struct {
	foldNowCalls []string
	onFoldNow    func(sessionID string)
}

func (f *fakeRefinerySidecar) StartSession(_, _, _, _ string) {}
func (f *fakeRefinerySidecar) StopSession(_ string)           {}
func (f *fakeRefinerySidecar) FoldNow(sessionID string) {
	f.foldNowCalls = append(f.foldNowCalls, sessionID)
	if f.onFoldNow != nil {
		f.onFoldNow(sessionID)
	}
}

// TestFreshDigestAfterForcedFold_ForcesFoldBeforeDeciding pins the ordering
// that makes the context-saver fallback deterministic instead of a race: the
// refinery must be asked to fold BEFORE the digest is checked. Autonomous
// folds are debounced >=30s, so a session that burns from the fold-start
// threshold to the relaunch threshold inside one debounce window reaches this
// decision with no digest, spawns a context-saver, and only then does the
// scheduled fold land — producing the handoff the relaunch actually reads
// while the saver's output is discarded. Checking first and folding never is
// what caused ~$2 of wasted context-saver churn in a real run.
func TestFreshDigestAfterForcedFold_ForcesFoldBeforeDeciding(t *testing.T) {
	t.Parallel()

	fake := &fakeRefinerySidecar{}
	s := New(Config{RefinerySidecar: fake})
	proc := &processInfo{
		sessionID:          "sess-fold",
		workflowInstanceID: "wfi-1",
		nodeID:             "verify-supply",
		startTime:          time.Now(),
	}

	// No pool wired, so freshSlotDigest can only answer false — the point of
	// this assertion is the call, not the verdict.
	if got := s.freshDigestAfterForcedFold(context.Background(), proc); got {
		t.Errorf("freshDigestAfterForcedFold = true with no digest store, want false")
	}
	if len(fake.foldNowCalls) != 1 || fake.foldNowCalls[0] != "sess-fold" {
		t.Fatalf("FoldNow calls = %v, want exactly one for sess-fold — without it the digest check races the fold debounce", fake.foldNowCalls)
	}
}

// TestFreshDigestAfterForcedFold_NilSidecarStillFallsBack covers the
// refinery-disabled and console/no-sidecar paths: the forced fold is an
// optimisation, never a dependency, so a nil sidecar must leave the
// context-saver fallback reachable rather than panic.
func TestFreshDigestAfterForcedFold_NilSidecarStillFallsBack(t *testing.T) {
	t.Parallel()

	s := New(Config{}) // no RefinerySidecar
	proc := &processInfo{
		sessionID:          "sess-nil",
		workflowInstanceID: "wfi-1",
		nodeID:             "n1",
		startTime:          time.Now(),
	}

	if got := s.freshDigestAfterForcedFold(context.Background(), proc); got {
		t.Errorf("freshDigestAfterForcedFold = true with no sidecar and no digest, want false (fallback must stay reachable)")
	}
}
