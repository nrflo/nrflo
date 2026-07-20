package spawner

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestCodexEngine_SendUserTurn_SkillExpandsProviderTextButPersistsRaw is the
// codex side of the Rule-6 dispatch acceptance test: a matched skill expands
// into turn/start's provider-visible text (expandSkillTurn), while the
// persisted user_input row keeps the ORIGINAL "/name args" text.
func TestCodexEngine_SendUserTurn_SkillExpandsProviderTextButPersistsRaw(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	paramsCh := make(chan json.RawMessage, 1)
	f.setOverride("turn/start", func(f *fakeAppServer, env rpcEnvelope) {
		paramsCh <- env.Params
		f.replyResult(*env.ID, `{"turn":{"id":"turn-skill-1"}}`)
	})

	skill := &SkillMatch{Name: "foo", Body: "BODY", Args: "x"}
	turn := UserTurn{Text: "/foo x", Skill: skill}
	if err := eng.SendUserTurn(context.Background(), turn); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}

	env := waitForOutbound(t, f, "turn/start", 2*time.Second)
	_ = env
	params := mustRecvParams(t, paramsCh)
	var p struct {
		Input []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"input"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		t.Fatalf("unmarshal turn/start params: %v", err)
	}
	if len(p.Input) != 1 {
		t.Fatalf("turn/start input = %+v, want exactly one block", p.Input)
	}
	want := expandSkillTurn(skill)
	if p.Input[0].Text != want {
		t.Errorf("turn/start text = %q, want expandSkillTurn output %q", p.Input[0].Text, want)
	}

	if n := countCategory(sink, "user_input"); n != 1 {
		t.Fatalf("user_input rows = %d, want 1", n)
	}
	var found bool
	for _, m := range sink.recordedMsgs {
		if m.category == "user_input" && m.content == "/foo x" {
			found = true
		}
	}
	if !found {
		t.Errorf("no user_input row with the ORIGINAL raw text %q: %+v", "/foo x", sink.recordedMsgs)
	}
}

// TestCodexEngine_SendUserTurn_NoSkill_TextPassesThroughUnexpanded is the
// baseline: a turn with no Skill sends turn.Text unmodified.
func TestCodexEngine_SendUserTurn_NoSkill_TextPassesThroughUnexpanded(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	paramsCh := make(chan json.RawMessage, 1)
	f.setOverride("turn/start", func(f *fakeAppServer, env rpcEnvelope) {
		paramsCh <- env.Params
		f.replyResult(*env.ID, `{"turn":{"id":"turn-noskill-1"}}`)
	})

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "plain text"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	params := mustRecvParams(t, paramsCh)
	var p struct {
		Input []struct {
			Text string `json:"text"`
		} `json:"input"`
	}
	_ = json.Unmarshal(params, &p)
	if len(p.Input) != 1 || p.Input[0].Text != "plain text" {
		t.Errorf("turn/start input = %+v, want unmodified %q", p.Input, "plain text")
	}
}
