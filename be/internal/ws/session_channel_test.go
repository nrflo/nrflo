package ws

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

func TestClientMessage_SubscribeSession_Decodes(t *testing.T) {
	var msg ClientMessage
	if err := json.Unmarshal([]byte(`{"action":"subscribe_session","session_id":"sess-1"}`), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Action != "subscribe_session" || msg.SessionID != "sess-1" {
		t.Fatalf("msg = %+v, want action=subscribe_session session_id=sess-1", msg)
	}
}

func TestBroadcastSession_DeliversOnlyToSubscribersOfThatSession(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	subscribed := newTestClient(hub, "sub")
	other := newTestClient(hub, "other-session")
	unrelated := newTestClient(hub, "unrelated")
	for _, c := range []*Client{subscribed, other, unrelated} {
		c.sessionSubs = make(map[string]bool)
		hub.Register(c)
	}
	hub.SubscribeSession(subscribed, "sess-A")
	hub.SubscribeSession(other, "sess-B")
	// unrelated has no session subscription at all.

	hub.BroadcastSession(&Event{Type: EventConsoleChatDelta, SessionID: "sess-A", Data: map[string]interface{}{"text": "hi"}})

	select {
	case msg := <-subscribed.send:
		var ev Event
		if err := json.Unmarshal(msg, &ev); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if ev.SessionID != "sess-A" {
			t.Errorf("SessionID = %q, want sess-A", ev.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: subscriber of sess-A did not receive the session event")
	}

	select {
	case msg := <-other.send:
		t.Fatalf("subscriber of a different session id received the event: %s", string(msg))
	case <-time.After(100 * time.Millisecond):
		// expected
	}

	select {
	case msg := <-unrelated.send:
		t.Fatalf("unsubscribed client received the session event: %s", string(msg))
	case <-time.After(100 * time.Millisecond):
		// expected
	}
}

func TestUnsubscribeSession_StopsDelivery(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "unsub-test")
	client.sessionSubs = make(map[string]bool)
	hub.Register(client)
	hub.SubscribeSession(client, "sess-X")
	hub.UnsubscribeSession(client, "sess-X")

	hub.BroadcastSession(&Event{Type: EventConsoleChatTurn, SessionID: "sess-X"})

	select {
	case msg := <-client.send:
		t.Fatalf("client received event after UnsubscribeSession: %s", string(msg))
	case <-time.After(150 * time.Millisecond):
		// expected
	}
}

func TestBroadcastSession_ProjectScopedSubscriberDoesNotReceiveIt(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	projClient := newTestClient(hub, "proj-only")
	hub.Register(projClient)
	hub.Subscribe(projClient, "proj-1", "")

	hub.BroadcastSession(&Event{Type: EventConsoleChatDelta, SessionID: "sess-A", ProjectID: "proj-1"})

	select {
	case msg := <-projClient.send:
		t.Fatalf("project-scoped subscriber received a session-scoped event: %s", string(msg))
	case <-time.After(150 * time.Millisecond):
		// expected: session events are a separate channel from project:ticket subscriptions
	}
}

func TestBroadcastSession_DoesNotAppendToEventLog(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	pool, err := openWSTestPool(t, dbPath)
	if err != nil {
		t.Fatalf("openWSTestPool: %v", err)
	}
	defer pool.Close()

	hub := NewHub(clock.Real())
	eventLog := repo.NewEventLogRepo(pool, clock.Real())
	hub.SetEventLog(eventLog)
	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "log-check")
	client.sessionSubs = make(map[string]bool)
	hub.Register(client)
	hub.SubscribeSession(client, "sess-log")

	hub.BroadcastSession(&Event{Type: EventConsoleChatDelta, SessionID: "sess-log", ProjectID: "proj-log"})

	select {
	case <-client.send:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for session broadcast delivery")
	}

	entries, err := eventLog.QuerySince("proj-log", "", 0, 100)
	if err != nil {
		t.Fatalf("QuerySince: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("event log entries after BroadcastSession = %d, want 0 (session events are ephemeral)", len(entries))
	}
}

func TestRemoveClientSubscriptions_ClearsSessionSubs(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client := newTestClient(hub, "cleanup-test")
	client.sessionSubs = make(map[string]bool)
	hub.Register(client)
	hub.SubscribeSession(client, "sess-cleanup")

	hub.mu.RLock()
	_, stillTracked := hub.sessionSubs["sess-cleanup"][client]
	hub.mu.RUnlock()
	if !stillTracked {
		t.Fatal("setup failed: client not tracked in sessionSubs before unregister")
	}

	hub.Unregister(client)

	// Hub.Run's unregister branch calls removeClientSubscriptions and then
	// closeSend, both under h.mu — so the send channel closing is a signal that
	// the session-sub cleanup has already happened. No polling, no sleep.
	select {
	case _, ok := <-client.send:
		if ok {
			t.Fatal("expected client.send to be closed by Unregister, got a message")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Unregister to close the client")
	}

	hub.mu.RLock()
	_, present := hub.sessionSubs["sess-cleanup"][client]
	hub.mu.RUnlock()
	if present {
		t.Fatal("client still present in hub.sessionSubs after Unregister")
	}
}
