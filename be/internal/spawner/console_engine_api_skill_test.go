package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/service"
)

// TestAPIConsoleEngine_SendUserTurn_SkillExpandsProviderTextButPersistsRaw is
// the api side of the Rule-6 dispatch acceptance test: a matched skill
// expands into the provider-visible request text (expandSkillTurn), while
// the persisted user_input row keeps the ORIGINAL "/name args" text.
func TestAPIConsoleEngine_SendUserTurn_SkillExpandsProviderTextButPersistsRaw(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	prov := &recordingProvider{}
	installFakeAPIProvider(t, prov, nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{
		Sink: sink,
		API:  APIEngineDeps{Pool: pool, Clock: clk},
	})
	if err := eng.Start(context.Background(), apiTestSpec("sess-skill")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	skill := &SkillMatch{Name: "foo", Body: "BODY", Args: "x"}
	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "/foo x", Skill: skill}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	_ = waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)

	reqs := prov.requests()
	if len(reqs) != 1 {
		t.Fatalf("provider Run calls = %d, want 1", len(reqs))
	}
	want := expandSkillTurn(skill)
	if got := lastUserText(t, reqs[0]); got != want {
		t.Errorf("provider request text = %q, want expandSkillTurn output %q", got, want)
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

// TestAPIConsoleEngine_SendUserTurn_NoSkill_TextPassesThroughUnexpanded is
// the baseline: a turn with no Skill sends turn.Text unmodified to the
// provider.
func TestAPIConsoleEngine_SendUserTurn_NoSkill_TextPassesThroughUnexpanded(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	prov := &recordingProvider{}
	installFakeAPIProvider(t, prov, nil)

	eng := newAPIConsoleEngine(EngineDeps{
		Sink: &testSink{},
		API:  APIEngineDeps{Pool: pool, Clock: clk},
	})
	if err := eng.Start(context.Background(), apiTestSpec("sess-noskill")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "plain text"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	_ = waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)

	reqs := prov.requests()
	if len(reqs) != 1 {
		t.Fatalf("provider Run calls = %d, want 1", len(reqs))
	}
	if got := lastUserText(t, reqs[0]); got != "plain text" {
		t.Errorf("provider request text = %q, want unmodified %q", got, "plain text")
	}
}
