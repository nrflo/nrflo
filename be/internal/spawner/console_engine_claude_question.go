package spawner

import (
	"fmt"
	"strings"
)

// AskUserQuestionTool is claude's interactive question tool. Its "execution"
// is a TUI picker inside the hidden console PTY that no console client can
// reach, so RequestApproval must never allow it — yolo and the session
// allowlist included (an allow would pin the turn on an unanswerable picker
// forever). The pending approval is surfaced as a question card instead
// (ApprovalRequest.Tool + Raw carry the structured questions) and resolved by
// AnswerQuestion, or by askUserQuestionRedirect on any allow-shaped decision.
const AskUserQuestionTool = "AskUserQuestion"

// askUserQuestionRedirect is the deny reason fed back to the model when a
// question cannot be answered through the card (allow-shaped decision from a
// consumer that doesn't render cards): the model re-asks in plain text and
// the conversation continues over normal turns.
const askUserQuestionRedirect = "nrflo: the interactive question picker is unavailable in this console; ask the question as plain text in your reply and end your turn — the user's answer arrives as the next message"

// answerReason wraps the human's answer into the PreToolUse deny reason the
// model receives as the tool feedback.
func answerReason(answer string) string {
	return "nrflo: the user answered directly (the interactive picker is unavailable in this console): " + answer + " — continue with this answer instead of re-asking"
}

// AnswerQuestion resolves a pending AskUserQuestion approval with the user's
// free-form answer: wire-mapped to a PreToolUse deny whose reason carries the
// answer, so the model continues with it immediately and no picker ever
// opens. Same drop-after-write + drop-wins discipline as ReplyApproval.
func (e *claudeEngine) AnswerQuestion(id, answer string) error {
	pa, ok := e.approvals.peek(id)
	if !ok {
		return fmt.Errorf("console engine: no pending approval %q", id)
	}
	if pa.toolName != AskUserQuestionTool {
		return fmt.Errorf("console engine: approval %q is %q, not a question", id, pa.toolName)
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return fmt.Errorf("console engine: answer required")
	}
	select {
	case pa.reply <- claudeApprovalResult{wire: "deny", reason: answerReason(answer)}:
	default:
		return fmt.Errorf("console engine: approval %q already answered or timed out", id)
	}
	if !e.approvals.drop(id) {
		return fmt.Errorf("console engine: approval %q already resolved", id)
	}
	e.emit(EngineEvent{Type: EventApprovalResolved, SessionID: e.sessionID(), ApprovalID: id, Decision: ApprovalAnswer, Text: answer})
	return nil
}
