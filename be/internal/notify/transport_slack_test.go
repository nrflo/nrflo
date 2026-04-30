package notify

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSlackTransport_Registered(t *testing.T) {
	tr := Get("slack")
	if tr == nil {
		t.Fatal("slack transport not registered")
	}
	if tr.Kind() != "slack" {
		t.Errorf("Kind() = %q, want slack", tr.Kind())
	}
}

func TestSlackTransport_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tr := Get("slack")
	err := tr.Send(&Notification{
		ChannelID: "c1",
		Kind:      "slack",
		Config:    map[string]interface{}{"webhook_url": server.URL},
		Body:      "hello",
	})
	if err != nil {
		t.Errorf("Send 200: got error %v, want nil", err)
	}
}

func TestSlackTransport_4xx_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	tr := Get("slack")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"webhook_url": server.URL},
		Body:   "hello",
	})
	if err == nil {
		t.Errorf("Send 400: expected error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "400") {
		t.Errorf("error = %q, want to contain status code 400", err.Error())
	}
}

func TestSlackTransport_5xx_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tr := Get("slack")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"webhook_url": server.URL},
		Body:   "hello",
	})
	if err == nil {
		t.Errorf("Send 500: expected error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want to contain status code 500", err.Error())
	}
}

func TestSlackTransport_MissingWebhookURL(t *testing.T) {
	tr := Get("slack")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{},
		Body:   "hello",
	})
	if err == nil {
		t.Errorf("Send missing webhook_url: expected error, got nil")
	}
}

func TestSlackTransport_EmptyWebhookURL(t *testing.T) {
	tr := Get("slack")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"webhook_url": ""},
		Body:   "hello",
	})
	if err == nil {
		t.Errorf("Send empty webhook_url: expected error, got nil")
	}
}
