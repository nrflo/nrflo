package spawner

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCodexEngine_QuestionRequestAndAnswer(t *testing.T) {
	eng, fake := startTestCodexEngine(t, &testSink{}, EngineSpec{Yolo: true})
	fake.feed(`{"id":42,"method":"item/tool/requestUserInput","params":{"threadId":"t1","turnId":"v1","itemId":"q1","isBlocking":true,"questions":[{"id":"scope","header":"Scope","question":"Pick scope","options":[{"label":"Small","description":"One file"}]},{"id":"ship","header":"Ship","question":"Ship now?","isSecret":true,"options":null}]}}`)
	ev := waitForEventType(t, eng.Events(), EventApprovalRequest, 2*time.Second)
	if ev.Approval == nil || ev.Approval.Tool != CodexQuestionTool || ev.ItemID != "q1" || !strings.Contains(string(ev.Approval.Raw), `"scope"`) {
		t.Fatalf("question event = %+v", ev)
	}
	if err := eng.AnswerQuestion("42", `{"answers":{"scope":{"answers":["Small"]}}}`); err == nil {
		t.Fatal("partial answer must be rejected without consuming the request")
	}
	answer := `{"answers":{"scope":{"answers":["Small"]},"ship":{"answers":["yes"]}}}`
	if err := eng.AnswerQuestion("42", answer); err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	reply := drainReplyFor(t, fake, "42", 2*time.Second)
	var got codexQuestionResponse
	if json.Unmarshal(reply.Result, &got) != nil || got.Answers["scope"].Answers[0] != "Small" || got.Answers["ship"].Answers[0] != "yes" {
		t.Fatalf("wire reply = %s", reply.Result)
	}
	resolved := waitForEventType(t, eng.Events(), EventApprovalResolved, 2*time.Second)
	if resolved.Decision != ApprovalAnswer || resolved.Text != "Small; [redacted]" {
		t.Fatalf("resolved = %+v", resolved)
	}
}

func TestCodexEngine_QuestionRejectsMalformedRequest(t *testing.T) {
	eng, fake := startTestCodexEngine(t, &testSink{}, EngineSpec{})
	fake.feed(`{"id":43,"method":"item/tool/requestUserInput","params":{"questions":[{"id":"same"},{"id":"same"}]}}`)
	reply := drainReplyFor(t, fake, "43", 2*time.Second)
	if reply.Error == nil || reply.Error.Code != -32602 {
		t.Fatalf("malformed request reply = %+v", reply)
	}
	if _, ok := eng.approvals.peek("43"); ok {
		t.Fatal("malformed request must not be pending")
	}
}
