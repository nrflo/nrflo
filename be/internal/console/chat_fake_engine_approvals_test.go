package console

import (
	"be/internal/spawner"
)

// The fakeConsoleEngine approval/question surface, split out of
// chat_fake_engine_test.go to keep both files under the 300-line cap.

// ReplyApproval mirrors the real engines' contract (console_engine_claude_approval.go,
// console_engine_codex_approval.go): after successfully forwarding the
// decision, the engine itself emits EventApprovalResolved — pumpChatEvents is
// the only thing that resolves the pending approval / pushes
// console_chat.approval_resolved, never ChatService.ReplyApproval directly.
func (f *fakeConsoleEngine) ReplyApproval(id string, decision spawner.ApprovalDecision) error {
	f.mu.Lock()
	if f.approveErr != nil {
		err := f.approveErr
		f.approveErr = nil
		f.mu.Unlock()
		return err
	}
	f.approvals = append(f.approvals, fakeApprovalCall{id: id, decision: decision})
	f.mu.Unlock()
	f.emit(spawner.EngineEvent{Type: spawner.EventApprovalResolved, ApprovalID: id, Decision: decision})
	return nil
}

// AnswerQuestion mirrors claudeEngine.AnswerQuestion's contract: on success
// the engine itself emits EventApprovalResolved with Decision=ApprovalAnswer
// and the answer as Text.
func (f *fakeConsoleEngine) AnswerQuestion(id, answer string) error {
	f.mu.Lock()
	if f.answerErr != nil {
		err := f.answerErr
		f.answerErr = nil
		f.mu.Unlock()
		return err
	}
	f.answers = append(f.answers, fakeAnswerCall{id: id, answer: answer})
	f.mu.Unlock()
	f.emit(spawner.EngineEvent{Type: spawner.EventApprovalResolved, ApprovalID: id, Decision: spawner.ApprovalAnswer, Text: answer})
	return nil
}

func (f *fakeConsoleEngine) answerCalls() []fakeAnswerCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeAnswerCall, len(f.answers))
	copy(out, f.answers)
	return out
}

func (f *fakeConsoleEngine) SessionApprovals() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.sessionAllowed))
	copy(out, f.sessionAllowed)
	return out
}

// RevokeSessionApproval records the tool and drops it from sessionAllowed,
// mirroring the claude/api engines' idempotent revoke.
func (f *fakeConsoleEngine) RevokeSessionApproval(tool string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked = append(f.revoked, tool)
	kept := f.sessionAllowed[:0]
	for _, t := range f.sessionAllowed {
		if t != tool {
			kept = append(kept, t)
		}
	}
	f.sessionAllowed = kept
	return nil
}
