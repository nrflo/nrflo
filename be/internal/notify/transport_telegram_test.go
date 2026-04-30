package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegramTransport_Registered(t *testing.T) {
	tr := Get("telegram")
	if tr == nil {
		t.Fatal("telegram transport not registered")
	}
	if tr.Kind() != "telegram" {
		t.Errorf("Kind() = %q, want telegram", tr.Kind())
	}
}

func TestTelegramTransport_OK(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	origBaseURL := TelegramBaseURL
	TelegramBaseURL = server.URL
	defer func() { TelegramBaseURL = origBaseURL }()

	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{
			"bot_token": "mytoken123",
			"chat_id":   "chat456",
		},
		Body: "hello world",
	})
	if err != nil {
		t.Errorf("Send OK: got error %v, want nil", err)
	}
	if !strings.HasSuffix(gotPath, "/sendMessage") {
		t.Errorf("path = %q, want suffix /sendMessage", gotPath)
	}
	if !strings.Contains(gotPath, "mytoken123") {
		t.Errorf("path = %q, want to contain bot token", gotPath)
	}
	if gotBody["chat_id"] != "chat456" {
		t.Errorf("chat_id = %q, want chat456", gotBody["chat_id"])
	}
	if gotBody["parse_mode"] != "MarkdownV2" {
		t.Errorf("parse_mode = %q, want MarkdownV2", gotBody["parse_mode"])
	}
}

func TestTelegramTransport_OkFalse_SurfacesDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found"}`))
	}))
	defer server.Close()

	origBaseURL := TelegramBaseURL
	TelegramBaseURL = server.URL
	defer func() { TelegramBaseURL = origBaseURL }()

	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"bot_token": "tok", "chat_id": "cid"},
		Body:   "msg",
	})
	if err == nil {
		t.Errorf("ok=false: expected error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("error = %q, want to contain 'chat not found'", err.Error())
	}
}

func TestTelegramTransport_4xx_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`forbidden`))
	}))
	defer server.Close()

	origBaseURL := TelegramBaseURL
	TelegramBaseURL = server.URL
	defer func() { TelegramBaseURL = origBaseURL }()

	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"bot_token": "tok", "chat_id": "cid"},
		Body:   "msg",
	})
	if err == nil {
		t.Errorf("4xx: expected error, got nil")
	}
}

func TestTelegramTransport_MissingBotToken(t *testing.T) {
	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"chat_id": "cid"},
		Body:   "msg",
	})
	if err == nil {
		t.Errorf("missing bot_token: expected error, got nil")
	}
}

func TestTelegramTransport_MissingChatID(t *testing.T) {
	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"bot_token": "tok"},
		Body:   "msg",
	})
	if err == nil {
		t.Errorf("missing chat_id: expected error, got nil")
	}
}
