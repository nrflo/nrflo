package consoleui

import (
	"context"
	"errors"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"be/internal/types"
)

// fakeDelete records calls and returns a canned error (nil = success).
type fakeDelete struct {
	calls []string
	err   error
}

func (f *fakeDelete) fn(_ context.Context, sessionID string) error {
	f.calls = append(f.calls, sessionID)
	return f.err
}

// twoResumeItems builds a picker item list with two resume rows followed by
// one brand branch, so tests can assert deletion only ever touches the
// targeted row.
func twoResumeItems() []list.Item {
	return selectionItems(Catalog{
		Sessions: []types.ConsoleSessionOption{
			{SessionID: "session-aaaa", Engine: "codex"},
			{SessionID: "session-bbbb", Engine: "codex"},
		},
		Engines: []types.ConsoleEngineOption{
			{ID: "claude", DisplayName: "Claude", Kind: "cli", Brand: "claude", Enabled: true,
				Models: []types.ConsoleModelOption{{ID: "sonnet-5", DisplayName: "Sonnet 5", Brand: "claude"}}},
		},
	})
}

func newDeleteTestModel(items []list.Item, del *fakeDelete) *selectionModel {
	model := &selectionModel{
		list: list.New(items, list.NewDefaultDelegate(), 80, 24),
		ctx:  context.Background(), deleteFn: del.fn, deleteArmed: -1,
	}
	model.list.Title = selectRootTitle
	return model
}

func keyPress(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

// runDeleteResult drives a tea.Cmd synchronously and feeds its message back
// through Update (no async goroutines), returning Update's own follow-up Cmd.
func runDeleteResult(t *testing.T, m *selectionModel, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a non-nil delete Cmd")
	}
	msg := cmd()
	if _, ok := msg.(deleteResultMsg); !ok {
		t.Fatalf("Cmd produced %T, want deleteResultMsg", msg)
	}
	_, follow := m.Update(msg)
	return follow
}

func TestDelete_ArmOnResumeRow(t *testing.T) {
	items := twoResumeItems()
	model := newDeleteTestModel(items, &fakeDelete{})

	model.Update(keyPress('d'))

	if model.deleteArmed != 0 {
		t.Fatalf("deleteArmed = %d, want 0", model.deleteArmed)
	}
	row := model.list.Items()[0].(selectionItem)
	if row.detail != "delete? d again · esc cancel" {
		t.Fatalf("armed row detail = %q", row.detail)
	}
}

func TestDelete_NoopOnNonResumeRow(t *testing.T) {
	items := twoResumeItems()
	model := newDeleteTestModel(items, &fakeDelete{})
	// Index 2 is the "Claude" brand branch (0,1 are the two resume rows).
	model.list.Select(2)

	model.Update(keyPress('d'))

	if model.deleteArmed != -1 {
		t.Fatalf("deleteArmed = %d, want -1 (brand row must not arm)", model.deleteArmed)
	}
}

func TestDelete_SecondPressConfirmsExactSession(t *testing.T) {
	items := twoResumeItems()
	del := &fakeDelete{}
	model := newDeleteTestModel(items, del)
	model.list.Select(1)

	model.Update(keyPress('d'))
	_, cmd := model.Update(keyPress('d'))

	runDeleteResult(t, model, cmd)

	if len(del.calls) != 1 || del.calls[0] != "session-bbbb" {
		t.Fatalf("deleteFn calls = %v, want [session-bbbb]", del.calls)
	}
}

func TestDelete_SuccessRemovesOnlyThatRow(t *testing.T) {
	items := twoResumeItems()
	del := &fakeDelete{}
	model := newDeleteTestModel(items, del)
	model.list.Select(0)

	model.Update(keyPress('d'))
	_, cmd := model.Update(keyPress('d'))
	runDeleteResult(t, model, cmd)

	remaining := model.list.Items()
	if len(remaining) != 2 {
		t.Fatalf("remaining item count = %d, want 2 (1 resume + 1 brand)", len(remaining))
	}
	row := remaining[0].(selectionItem)
	if row.selection.ResumeID != "session-bbbb" {
		t.Fatalf("surviving resume row = %+v, want session-bbbb", row.selection)
	}
}

// TestDelete_RemovingLastResumeRowLeavesBrandsOnly deletes both resume rows in
// turn and checks the picker collapses to just the brand branches.
func TestDelete_RemovingLastResumeRowLeavesBrandsOnly(t *testing.T) {
	items := twoResumeItems()
	del := &fakeDelete{}
	model := newDeleteTestModel(items, del)

	for i := 0; i < 2; i++ {
		model.list.Select(0)
		model.Update(keyPress('d'))
		_, cmd := model.Update(keyPress('d'))
		runDeleteResult(t, model, cmd)
	}

	remaining := model.list.Items()
	if len(remaining) != 1 {
		t.Fatalf("remaining item count = %d, want 1 (brand only)", len(remaining))
	}
	if _, ok := remaining[0].(selectionItem); !ok || isResumeRow(remaining[0]) {
		t.Fatalf("surviving row = %+v, want the non-resume brand branch", remaining[0])
	}
}

func TestDelete_EscCancelsArmingAndRestoresRow(t *testing.T) {
	items := twoResumeItems()
	del := &fakeDelete{}
	model := newDeleteTestModel(items, del)
	original := model.list.Items()[0].(selectionItem)

	model.Update(keyPress('d'))
	model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	if model.deleteArmed != -1 {
		t.Fatalf("deleteArmed = %d, want -1 after esc", model.deleteArmed)
	}
	row := model.list.Items()[0].(selectionItem)
	if row.detail != original.detail {
		t.Fatalf("row detail = %q, want restored %q", row.detail, original.detail)
	}
	if len(del.calls) != 0 {
		t.Fatalf("deleteFn must not be called on cancel, calls = %v", del.calls)
	}
}

// TestDelete_NavigationDisarmsAndRestores mirrors esc but via any other key
// (the "default" branch of Update's key switch).
func TestDelete_NavigationDisarmsAndRestores(t *testing.T) {
	items := twoResumeItems()
	del := &fakeDelete{}
	model := newDeleteTestModel(items, del)
	original := model.list.Items()[0].(selectionItem)

	model.Update(keyPress('d'))
	model.Update(keyPress('j'))

	if model.deleteArmed != -1 {
		t.Fatalf("deleteArmed = %d, want -1 after navigation key", model.deleteArmed)
	}
	row := model.list.Items()[0].(selectionItem)
	if row.detail != original.detail {
		t.Fatalf("row detail = %q, want restored %q", row.detail, original.detail)
	}
}

func TestDelete_ErrorKeepsRowAndSetsStatusMessage(t *testing.T) {
	items := twoResumeItems()
	del := &fakeDelete{err: errors.New("close failed: engine busy")}
	model := newDeleteTestModel(items, del)
	original := model.list.Items()[0].(selectionItem)

	model.Update(keyPress('d'))
	_, cmd := model.Update(keyPress('d'))
	follow := runDeleteResult(t, model, cmd)
	if follow == nil {
		t.Fatal("expected a non-nil follow-up Cmd carrying the status message on error")
	}

	if len(del.calls) != 1 || del.calls[0] != "session-aaaa" {
		t.Fatalf("deleteFn calls = %v, want [session-aaaa]", del.calls)
	}
	remaining := model.list.Items()
	if len(remaining) != 3 {
		t.Fatalf("remaining item count = %d, want 3 (row restored, not removed)", len(remaining))
	}
	row := remaining[0].(selectionItem)
	if row.selection.ResumeID != original.selection.ResumeID {
		t.Fatalf("restored row = %+v, want %+v", row.selection, original.selection)
	}
	if model.deleteArmed != -1 {
		t.Fatalf("deleteArmed = %d, want -1 after error result", model.deleteArmed)
	}
}
