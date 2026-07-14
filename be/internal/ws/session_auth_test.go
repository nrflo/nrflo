package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"be/internal/clock"

	"github.com/gorilla/websocket"
)

// dialTestWS serves h over an httptest server and returns a live client
// connection to it — the only way to exercise Client.ReadPump's action
// handling (and therefore the subscribe_session gate) end to end.
func dialTestWS(t *testing.T, h *Handler) *websocket.Conn {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// subscribeSessionAck sends one subscribe_session action and returns the ack's
// action field.
func subscribeSessionAck(t *testing.T, conn *websocket.Conn, sessionID string) string {
	t.Helper()
	if err := conn.WriteJSON(ClientMessage{Action: "subscribe_session", SessionID: sessionID}); err != nil {
		t.Fatalf("write subscribe_session: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	var ack map[string]string
	if err := json.Unmarshal(raw, &ack); err != nil {
		t.Fatalf("unmarshal ack: %v", err)
	}
	return ack["action"]
}

func sessionSubCount(h *Hub, sessionID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.sessionSubs[sessionID])
}

// TestSubscribeSession_AuthorizerGatesSubscription asserts a client can only
// join the session channel of a session the authorizer allows. Session
// channels carry live console-chat assistant output that the REST routes gate
// behind admin/service-principal/own-bearer, so the WS side must apply the
// same predicate rather than trusting the client-supplied session id.
func TestSubscribeSession_AuthorizerGatesSubscription(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	h := NewHandler(hub)
	h.SetSessionAuthorizer(func(*http.Request) SessionAuthorizer {
		return func(sessionID string) bool { return sessionID == "sess-allowed" }
	})
	conn := dialTestWS(t, h)

	if got := subscribeSessionAck(t, conn, "sess-denied"); got != "session_subscription_denied" {
		t.Errorf("ack for an unauthorized session = %q, want session_subscription_denied", got)
	}
	if got := subscribeSessionAck(t, conn, "sess-allowed"); got != "subscribed_session" {
		t.Errorf("ack for an authorized session = %q, want subscribed_session", got)
	}

	if n := sessionSubCount(hub, "sess-denied"); n != 0 {
		t.Errorf("hub tracks %d subscriber(s) for the denied session, want 0", n)
	}
	if n := sessionSubCount(hub, "sess-allowed"); n != 1 {
		t.Errorf("hub tracks %d subscriber(s) for the allowed session, want 1", n)
	}
}

// TestSubscribeSession_NoAuthorizerConfigured_Denies asserts the gate fails
// closed: a Handler with no authorizer wired rejects every subscribe_session
// rather than defaulting to the pre-authorization behaviour of joining any
// session id a client names.
func TestSubscribeSession_NoAuthorizerConfigured_Denies(t *testing.T) {
	hub := NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	conn := dialTestWS(t, NewHandler(hub))

	if got := subscribeSessionAck(t, conn, "sess-any"); got != "session_subscription_denied" {
		t.Errorf("ack with no authorizer configured = %q, want session_subscription_denied", got)
	}
	if n := sessionSubCount(hub, "sess-any"); n != 0 {
		t.Errorf("hub tracks %d subscriber(s) with no authorizer configured, want 0", n)
	}
}
