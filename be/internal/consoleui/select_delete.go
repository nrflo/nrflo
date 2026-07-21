package consoleui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// deleteKey advertises the picker's two-press-confirm delete affordance in
// the footer; only shown (via AdditionalShortHelpKeys) when the highlighted
// row is a resume row.
var deleteKey = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete"))

// deleteConfirmKey replaces deleteKey in the footer once a row is armed.
var deleteConfirmKey = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "confirm · esc cancel"))

// isResumeRow reports whether item is a resumable live-chat leaf — the only
// rows deletable from the picker (brand/model/mode branches are not).
func isResumeRow(item list.Item) bool {
	sel, ok := item.(selectionItem)
	return ok && sel.selection.ResumeID != ""
}

// deleteResultMsg carries a background deleteFn call's outcome back into
// Update. original is the pre-arm row snapshot captured when the delete was
// confirmed, used to restore the correct row on error even if the user has
// since armed/confirmed a different row while this call was in flight.
type deleteResultMsg struct {
	index     int
	sessionID string
	original  selectionItem
	err       error
}

// armDelete marks the currently highlighted resume row as pending a confirm
// press, swapping its detail line for a confirm prompt while preserving the
// original item so cancelDelete can restore it exactly.
func (m *selectionModel) armDelete() tea.Cmd {
	item, ok := m.list.SelectedItem().(selectionItem)
	if !ok || item.selection.ResumeID == "" {
		return nil
	}
	index := m.list.Index()
	m.deleteArmed = index
	m.deleteOriginal = item
	confirmItem := item
	confirmItem.detail = "delete? d again · esc cancel"
	return m.list.SetItem(index, confirmItem)
}

// cancelDelete disarms any pending confirm, restoring the original row.
func (m *selectionModel) cancelDelete() tea.Cmd {
	if m.deleteArmed < 0 {
		return nil
	}
	index := m.deleteArmed
	original := m.deleteOriginal
	m.deleteArmed = -1
	m.deleteOriginal = selectionItem{}
	return m.list.SetItem(index, original)
}

// confirmDelete runs the injected deleteFn for the armed row asynchronously,
// returning its outcome as a deleteResultMsg. It does not itself mutate the
// list — applyDeleteResult does that once the call completes.
// confirmDelete dispatches the async deleteFn for the armed row and
// immediately clears deleteArmed (leaving deletePending set), so the user is
// free to arm/confirm a different row while this call is still in flight
// without corrupting either row's state when the result lands.
func (m *selectionModel) confirmDelete() tea.Cmd {
	if m.deleteArmed < 0 || m.deleteFn == nil || m.deletePending {
		return nil
	}
	index := m.deleteArmed
	original := m.deleteOriginal
	sid := original.selection.ResumeID
	deleteFn := m.deleteFn
	ctx := m.ctx
	m.deleteArmed = -1
	m.deleteOriginal = selectionItem{}
	m.deletePending = true
	return func() tea.Msg {
		err := deleteFn(ctx, sid)
		return deleteResultMsg{index: index, sessionID: sid, original: original, err: err}
	}
}

// applyDeleteResult removes the deleted row on success (only when it still
// matches the session id that was deleted — the list may have shifted while
// the call was in flight) or restores the row from msg's own captured
// snapshot and surfaces the error otherwise. It never touches the model's
// current deleteArmed/deleteOriginal, which may by now belong to a different
// row armed after this delete was confirmed.
func (m *selectionModel) applyDeleteResult(msg deleteResultMsg) tea.Cmd {
	m.deletePending = false
	items := m.list.Items()
	if msg.err != nil {
		if msg.index >= 0 && msg.index < len(items) {
			if row, ok := items[msg.index].(selectionItem); ok && row.selection.ResumeID == msg.sessionID {
				m.list.SetItem(msg.index, msg.original)
				return m.list.NewStatusMessage("delete failed: " + msg.err.Error())
			}
		}
		for i, it := range items {
			if row, ok := it.(selectionItem); ok && row.selection.ResumeID == msg.sessionID {
				m.list.SetItem(i, msg.original)
				break
			}
		}
		return m.list.NewStatusMessage("delete failed: " + msg.err.Error())
	}
	if msg.index >= 0 && msg.index < len(items) {
		if row, ok := items[msg.index].(selectionItem); ok && row.selection.ResumeID == msg.sessionID {
			m.list.RemoveItem(msg.index)
			return nil
		}
	}
	for i, it := range items {
		if row, ok := it.(selectionItem); ok && row.selection.ResumeID == msg.sessionID {
			m.list.RemoveItem(i)
			return nil
		}
	}
	return nil
}
