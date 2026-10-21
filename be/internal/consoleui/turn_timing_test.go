package consoleui

import (
	"testing"
	"time"
)

func TestTurnTiming(t *testing.T) {
	m := &model{}
	start := time.Date(2026, 10, 21, 12, 32, 0, 0, time.Local)
	m.recordTurnTiming("idle", start)
	if len(m.turnSummaries) != 0 {
		t.Fatal("idle without a known start fabricated a summary")
	}
	m.recordTurnTiming("running", start)
	m.recordTurnTiming("running", start.Add(10*time.Second))
	m.recordTurnTiming("idle", start.Add(22*time.Second))
	m.recordTurnTiming("idle", start.Add(23*time.Second))
	if len(m.turnSummaries) != 1 || m.turnSummaries[0] != "Worked for 22s · 12:32" {
		t.Fatalf("summaries = %v", m.turnSummaries)
	}
	if !m.turnStarted.IsZero() {
		t.Fatal("completed turn kept its start time")
	}
	m.recordTurnTiming("running", start.Add(time.Minute))
	m.recordTurnTiming("idle", start.Add(2*time.Minute+5*time.Second))
	if len(m.turnSummaries) != 2 || m.turnSummaries[1] != "Worked for 1m05s · 12:34" {
		t.Fatalf("next turn summaries = %v", m.turnSummaries)
	}
}
