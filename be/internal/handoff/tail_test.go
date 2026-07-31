package handoff

import (
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/repo"
)

// TestRenderTail_DropsSystemNoticeRows verifies renderTail excludes rows
// categorized model.MsgCategorySystemNotice (idle/unknown Notification hook
// noise) from the rendered tail, alongside the existing tailDropPrefixes
// exploration-noise filter, while keeping ordinary rows.
func TestRenderTail_DropsSystemNoticeRows(t *testing.T) {
	msgs := []repo.TailMessage{
		{Category: "assistant", Content: "kept assistant line"},
		{Category: model.MsgCategorySystemNotice, Content: "dropped notice line"},
		{Category: "tool", Content: "[Read] /x/y.go"}, // dropped via tailDropPrefixes, not category
	}
	tail := renderTail(msgs)
	if !strings.Contains(tail, "kept assistant line") {
		t.Errorf("renderTail = %q, want it to contain the assistant row", tail)
	}
	if strings.Contains(tail, "dropped notice line") {
		t.Errorf("renderTail = %q, want it to NOT contain the system_notice row", tail)
	}
	if strings.Contains(tail, "[Read]") {
		t.Errorf("renderTail = %q, want it to NOT contain the [Read] exploration row", tail)
	}
}

// TestRenderTail_KeepsTaskNotificationRows verifies a task_notification row
// is rendered like any ordinary category — only system_notice is special-cased.
func TestRenderTail_KeepsTaskNotificationRows(t *testing.T) {
	msgs := []repo.TailMessage{
		{Category: model.MsgCategoryTaskNotification, Content: "task envelope content"},
	}
	tail := renderTail(msgs)
	if !strings.Contains(tail, "task envelope content") {
		t.Errorf("renderTail = %q, want it to contain the task_notification row", tail)
	}
}

// TestRenderTail_AllRowsDropped_ReturnsEmpty verifies an all-system_notice
// input renders to "" rather than an empty "## Recent Uncompressed Context"
// section header.
func TestRenderTail_AllRowsDropped_ReturnsEmpty(t *testing.T) {
	msgs := []repo.TailMessage{
		{Category: model.MsgCategorySystemNotice, Content: "notice one"},
		{Category: model.MsgCategorySystemNotice, Content: "notice two"},
	}
	if tail := renderTail(msgs); tail != "" {
		t.Errorf("renderTail = %q, want empty string", tail)
	}
}
