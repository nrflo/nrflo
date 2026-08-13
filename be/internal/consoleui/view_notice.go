package consoleui

import "strings"

// taskNotificationSummaryMax bounds the collapsed one-line summary so a
// large delegate result never blows out a single printed row before
// fitWidth wraps it.
const taskNotificationSummaryMax = 200

// collapseTaskNotification renders a Claude Code CLI harness
// <task-notification> envelope (delivered as a UserPromptSubmit prompt when
// a backgrounded MCP get_delegation call resolves) as a single muted line:
// "task · <task-id> <status> — <summary>". Falls back to a truncated first
// line of content when the expected tags aren't found, so a shape change in
// the harness's envelope degrades gracefully instead of rendering nothing.
func collapseTaskNotification(content string) string {
	taskID := firstNonEmpty(extractTag(content, "task_id"), extractTag(content, "task-id"), extractTag(content, "id"))
	status := extractTag(content, "status")
	summary := firstNonEmpty(extractTag(content, "summary"), extractTag(content, "result"), extractTag(content, "findings"))

	if taskID == "" && status == "" && summary == "" {
		return "task · " + truncateSummary(firstLine(content), taskNotificationSummaryMax)
	}

	var b strings.Builder
	b.WriteString("task")
	if taskID != "" {
		b.WriteString(" · " + taskID)
	}
	if status != "" {
		b.WriteString(" " + status)
	}
	if summary != "" {
		b.WriteString(" — " + truncateSummary(summary, taskNotificationSummaryMax))
	}
	return b.String()
}

// serverNoticePrefix marks a ChatNotifier wake-up. The server owns this
// literal (be/internal/console/chat_notify.go); it is stripped for display
// because the category already conveys authorship.
const serverNoticePrefix = "[nrflo] "

// serverNoticeSummaryMax bounds the collapsed line.
const serverNoticeSummaryMax = 160

// collapseServerNotice renders a category="system_turn" row — a server
// -authored wake-up telling the model a delegation or run finished — as one
// muted line: "nrflo · Delegation <id> finished". The trailing clause of every
// notice is an instruction addressed to the model ("collect the results with
// get_delegation…"), useless to a human reading the transcript, so the line is
// cut at the em dash that introduces it. A notice without one (a failure,
// which carries its error message after a colon) is kept whole.
func collapseServerNotice(content string) string {
	body := strings.TrimPrefix(strings.TrimSpace(content), serverNoticePrefix)
	if idx := strings.Index(body, " — "); idx > 0 {
		body = body[:idx]
	}
	return "nrflo · " + truncateSummary(firstLine(body), serverNoticeSummaryMax)
}

// extractTag returns the trimmed text between the first <tag>...</tag> (or
// self-describing <tag ...>...</tag>) pair, or "" if the tag isn't present.
func extractTag(content, tag string) string {
	open := "<" + tag + ">"
	idx := strings.Index(content, open)
	if idx < 0 {
		return ""
	}
	rest := content[idx+len(open):]
	close := "</" + tag + ">"
	end := strings.Index(rest, close)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstLine(content string) string {
	content = strings.TrimSpace(content)
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		return content[:idx]
	}
	return content
}

func truncateSummary(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
