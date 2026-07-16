package spawner

import (
	"reflect"
	"testing"
)

// TestSessionAllowlists_ListAndRevoke covers the claude/api engines' shared
// allowlist contract: allowForSession → listed (sorted) → revoke → the next
// allowedForSession check asks the human again.
func TestSessionAllowlists_ListAndRevoke(t *testing.T) {
	t.Run("claude", func(t *testing.T) {
		p := newClaudeApprovals()
		p.allowForSession("edit_file")
		p.allowForSession("bash")
		if got := p.listAllowed(); !reflect.DeepEqual(got, []string{"bash", "edit_file"}) {
			t.Fatalf("listAllowed = %v, want [bash edit_file]", got)
		}
		p.revoke("bash")
		if p.allowedForSession("bash") {
			t.Error("bash still allowed after revoke")
		}
		if !p.allowedForSession("edit_file") {
			t.Error("edit_file lost its approval on an unrelated revoke")
		}
		p.revoke("bash") // idempotent
	})

	t.Run("api", func(t *testing.T) {
		p := newAPIEngineApprovals()
		p.allowForSession("edit_file")
		p.allowForSession("bash")
		if got := p.listAllowed(); !reflect.DeepEqual(got, []string{"bash", "edit_file"}) {
			t.Fatalf("listAllowed = %v, want [bash edit_file]", got)
		}
		p.revoke("bash")
		if p.allowedForSession("bash") {
			t.Error("bash still allowed after revoke")
		}
		if !p.allowedForSession("edit_file") {
			t.Error("edit_file lost its approval on an unrelated revoke")
		}
	})
}
