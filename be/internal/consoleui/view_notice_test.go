package consoleui

import (
	"strings"
	"testing"
)

// TestCollapseTaskNotification covers the envelope-collapse table: full
// tag set, partial tags, and the no-tags-found fallback to a truncated
// first line.
func TestCollapseTaskNotification(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "full tag set via task_id/status/summary",
			content: "<task-notification><task_id>t-1</task_id><status>done</status><summary>fixed the bug</summary></task-notification>",
			want:    "task · t-1 done — fixed the bug",
		},
		{
			name:    "alt id tag and result fallback for summary",
			content: "<task-notification><id>t-2</id><result>ran tests</result></task-notification>",
			want:    "task · t-2 — ran tests",
		},
		{
			name:    "status only, no id or summary",
			content: "<task-notification><status>failed</status></task-notification>",
			want:    "task failed",
		},
		{
			name:    "no recognized tags falls back to truncated first line",
			content: "just some free-form text\nsecond line",
			want:    "task · just some free-form text",
		},
		{
			name:    "empty content falls back to empty first line",
			content: "",
			want:    "task · ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := collapseTaskNotification(tt.content); got != tt.want {
				t.Errorf("collapseTaskNotification(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

// TestCollapseTaskNotification_TruncatesLongSummary verifies a summary over
// taskNotificationSummaryMax is truncated with a trailing ellipsis rather
// than blowing out the collapsed line.
func TestCollapseTaskNotification_TruncatesLongSummary(t *testing.T) {
	long := strings.Repeat("x", taskNotificationSummaryMax+50)
	content := "<task-notification><task_id>t-3</task_id><summary>" + long + "</summary></task-notification>"
	got := collapseTaskNotification(content)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("collapseTaskNotification long summary = %q, want it to end with an ellipsis", got)
	}
	if len(got) > len("task · t-3 — ")+taskNotificationSummaryMax+len("…")+5 {
		t.Errorf("collapseTaskNotification long summary length = %d, want it bounded near taskNotificationSummaryMax", len(got))
	}
}

// TestRenderMessage_SystemNotice_CollapsesToEmptyString verifies
// renderMessage's "system_notice" branch returns "" (skipped by
// printNewMessages / does not consume the print-once watermark oddly).
func TestRenderMessage_SystemNotice_CollapsesToEmptyString(t *testing.T) {
	got := renderMessage(Message{Category: "system_notice", Content: "Claude is waiting for your input"}, 80)
	if got != "" {
		t.Errorf("renderMessage(system_notice) = %q, want empty string", got)
	}
}

// TestRenderMessage_TaskNotification_RendersMutedCollapsedLine verifies
// renderMessage's "task_notification" branch renders the collapsed one-liner
// through fitWidth (never over-wide, no literal tabs) rather than falling
// through to the glamour-rendered default branch.
func TestRenderMessage_TaskNotification_RendersMutedCollapsedLine(t *testing.T) {
	const width = 60
	content := "<task-notification><task_id>t-9</task_id><status>done</status><summary>all good</summary></task-notification>"
	got := renderMessage(Message{Category: "task_notification", Content: content}, width)
	if !strings.Contains(got, "t-9") || !strings.Contains(got, "done") || !strings.Contains(got, "all good") {
		t.Errorf("renderMessage(task_notification) = %q, want it to contain the collapsed task_id/status/summary", got)
	}
	if strings.Contains(got, "<task-notification>") {
		t.Errorf("renderMessage(task_notification) = %q, want the raw envelope tags stripped", got)
	}
}
