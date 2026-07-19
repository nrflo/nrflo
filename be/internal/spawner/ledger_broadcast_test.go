package spawner

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/ws"
)

// TestBroadcastLedgerEpoch_EmitsEventWithEpochSummary verifies
// broadcastLedgerEpoch fires an agent.context_ledger WS event carrying the
// entry count and totals-by-kind, then debounces a second call until the
// window elapses (via globalLedgerStore, since production wiring is not
// injectable).
func TestBroadcastLedgerEpoch_EmitsEventWithEpochSummary(t *testing.T) {
	t.Parallel()
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "client-ledger-broadcast")
	hub.Register(client)
	hub.Subscribe(client, "proj-ledger-bcast", "ticket-ledger-bcast")

	s := New(Config{WSHub: hub, Clock: clock.Real()})

	sessionID := "sess-ledger-broadcast"
	t.Cleanup(func() { globalLedgerStore.drop(sessionID) })
	l := globalLedgerStore.get(sessionID)
	l.append(LedgerKindDialog, 40, "", "", false)
	l.append(LedgerKindFileRead, 60, "/repo/a.txt", "", false)

	proc := &processInfo{
		sessionID:    sessionID,
		projectID:    "proj-ledger-bcast",
		ticketID:     "ticket-ledger-bcast",
		workflowName: "feature",
	}

	s.broadcastLedgerEpoch(proc)

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentContextLedger {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventAgentContextLedger)
		}
		if got := event.Data["session_id"]; got != sessionID {
			t.Errorf("data.session_id = %v, want %q", got, sessionID)
		}
		if got, _ := event.Data["entry_count"].(float64); int(got) != 2 {
			t.Errorf("data.entry_count = %v, want 2", event.Data["entry_count"])
		}
		if got, _ := event.Data["total_tokens"].(float64); int(got) != 100 {
			t.Errorf("data.total_tokens = %v, want 100", event.Data["total_tokens"])
		}
		totals, ok := event.Data["totals_by_kind"].(map[string]interface{})
		if !ok {
			t.Fatalf("data.totals_by_kind = %T, want map", event.Data["totals_by_kind"])
		}
		if got, _ := totals[string(LedgerKindFileRead)].(float64); int(got) != 60 {
			t.Errorf("totals_by_kind[file_read] = %v, want 60", totals[string(LedgerKindFileRead)])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for agent.context_ledger event")
	}

	// Immediately re-broadcasting within the debounce window must not fire
	// a second event.
	s.broadcastLedgerEpoch(proc)
	select {
	case msg := <-ch:
		t.Fatalf("unexpected second broadcast within debounce window: %s", msg)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestBroadcastLedgerEpoch_UnknownSessionIsNoOp verifies a session with no
// tracked ledger never broadcasts (shouldBroadcast/epochSummary both report
// ok=false for it).
func TestBroadcastLedgerEpoch_UnknownSessionIsNoOp(t *testing.T) {
	t.Parallel()
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "client-ledger-broadcast-noop")
	hub.Register(client)
	hub.Subscribe(client, "proj-ledger-noop", "ticket-ledger-noop")

	s := New(Config{WSHub: hub, Clock: clock.Real()})
	proc := &processInfo{
		sessionID:    "sess-ledger-never-tracked",
		projectID:    "proj-ledger-noop",
		ticketID:     "ticket-ledger-noop",
		workflowName: "feature",
	}

	s.broadcastLedgerEpoch(proc)

	select {
	case msg := <-ch:
		t.Fatalf("unexpected broadcast for a session with no ledger: %s", msg)
	case <-time.After(100 * time.Millisecond):
	}
}
