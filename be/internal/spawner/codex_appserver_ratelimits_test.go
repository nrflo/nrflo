package spawner

import (
	"encoding/json"
	"testing"

	"be/internal/service"
)

type rateLimitTestSink struct {
	testSink
	calls              int
	fiveHour, sevenDay service.RateLimitWindow
}

func (s *rateLimitTestSink) UpdateRateLimits(sessionID string, fiveHour, sevenDay service.RateLimitWindow) {
	s.calls++
	s.fiveHour, s.sevenDay = fiveHour, sevenDay
}

func TestDispatchAppServerEvent_RecordsCodexRateLimitsByDuration(t *testing.T) {
	sink := &rateLimitTestSink{}
	env := rpcEnvelope{Method: "account/rateLimits/updated", Params: json.RawMessage(`{"rateLimits":{"rateLimitReachedType":null,
		"primary":{"usedPercent":97,"windowDurationMins":10080,"resetsAt":1798761600},
		"secondary":{"usedPercent":12,"windowDurationMins":300,"resetsAt":1798700000}}}`)}

	sig := dispatchAppServerEvent("s1", env, sink, 0, nil)

	if sig.rateLimited {
		t.Fatal("routine usage update must not signal rate limit")
	}
	if sink.calls != 1 {
		t.Fatalf("UpdateRateLimits calls = %d, want 1", sink.calls)
	}
	if got := *sink.sevenDay.UsedPercentage; got != 97 || sink.sevenDay.ResetsAt != "2027-01-01T00:00:00Z" {
		t.Errorf("sevenDay = %v %q", got, sink.sevenDay.ResetsAt)
	}
	if got := *sink.fiveHour.UsedPercentage; got != 12 {
		t.Errorf("fiveHour pct = %v, want 12", got)
	}
}

func TestDispatchAppServerEvent_SkipsUnknownRateLimitWindows(t *testing.T) {
	sink := &rateLimitTestSink{}
	env := rpcEnvelope{Method: "account/rateLimits/updated", Params: json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":5,"windowDurationMins":60},"secondary":{"usedPercent":5}}}`)}

	dispatchAppServerEvent("s1", env, sink, 0, nil)

	if sink.calls != 0 {
		t.Fatalf("UpdateRateLimits calls = %d, want 0", sink.calls)
	}
}
