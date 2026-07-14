package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/model"
)

const entityParseBody = `{"ok":false,"description":"Bad Request: can't parse entities: Can't find end of Bold entity at byte offset 42"}`

func TestTelegramTransport_EntityParseError_FallsBackToPlaintext(t *testing.T) {
	var calls int
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		var b map[string]any
		_ = json.Unmarshal(raw, &b)
		requests = append(requests, b)
		w.Header().Set("Content-Type", "application/json")
		if _, ok := b["parse_mode"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(entityParseBody))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	origBaseURL := TelegramBaseURL
	TelegramBaseURL = server.URL
	defer func() { TelegramBaseURL = origBaseURL }()

	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"bot_token": "tok", "chat_id": "cid"},
		Body:   `*unbalanced entity`,
	})
	if err != nil {
		t.Fatalf("Send: got error %v, want nil", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 requests (MarkdownV2 then plaintext), got %d", calls)
	}
	if _, ok := requests[1]["parse_mode"]; ok {
		t.Errorf("second request must have NO parse_mode key, got %v", requests[1]["parse_mode"])
	}
	gotText, _ := requests[1]["text"].(string)
	if strings.Contains(gotText, `\`) {
		t.Errorf("second request text still has escapes: %q", gotText)
	}
	if gotText != "unbalanced entity" && !strings.Contains(gotText, "unbalanced entity") {
		t.Errorf("second request text = %q, want stripped content", gotText)
	}
}

func TestTelegramTransport_PlaintextFallbackAlsoFails_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(entityParseBody))
	}))
	defer server.Close()

	origBaseURL := TelegramBaseURL
	TelegramBaseURL = server.URL
	defer func() { TelegramBaseURL = origBaseURL }()

	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"bot_token": "tok", "chat_id": "cid"},
		Body:   `*unbalanced entity`,
	})
	if err == nil {
		t.Fatal("expected error when both MarkdownV2 and plaintext fallback fail")
	}
}

func TestTelegramTransport_NonEntityError_NoFallback(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found"}`))
	}))
	defer server.Close()

	origBaseURL := TelegramBaseURL
	TelegramBaseURL = server.URL
	defer func() { TelegramBaseURL = origBaseURL }()

	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"bot_token": "tok", "chat_id": "cid"},
		Body:   "hello",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls != 1 {
		t.Errorf("non-entity error must not retry: expected 1 call, got %d", calls)
	}
}

func TestTelegramTransport_MultiChunk_FallbackIsPerChunk(t *testing.T) {
	// Two paragraphs, each its own chunk once split. Only the chunk
	// containing "BADCHUNK" is rejected with an entity-parse error.
	p1 := strings.Repeat("a", 3500)
	p2 := "BADCHUNK" + strings.Repeat("b", 3500)
	body := p1 + "\n\n" + p2

	var texts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b map[string]any
		_ = json.Unmarshal(raw, &b)
		text, _ := b["text"].(string)
		texts = append(texts, text)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(text, "BADCHUNK") {
			if _, ok := b["parse_mode"]; ok {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(entityParseBody))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	origBaseURL := TelegramBaseURL
	TelegramBaseURL = server.URL
	defer func() { TelegramBaseURL = origBaseURL }()

	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"bot_token": "tok", "chat_id": "cid"},
		Body:   body,
	})
	if err != nil {
		t.Fatalf("Send: got error %v, want nil", err)
	}
	if len(texts) < 3 {
		t.Fatalf("expected at least 3 requests (good chunk + bad chunk x2), got %d", len(texts))
	}
	joined := strings.Join(texts, "")
	if !strings.Contains(joined, "BADCHUNK") {
		t.Errorf("bad chunk content lost, joined = %q", joined[:min(50, len(joined))])
	}
}

func TestTelegramTransport_Acceptance_MalformedTemplateStillDelivers(t *testing.T) {
	summary := strings.Repeat("x", 5900) + "**bold**" + "*" + "`" + "_x"
	template := `*unbalanced _entities ${summary}`

	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b map[string]any
		_ = json.Unmarshal(raw, &b)
		requests = append(requests, b)
		w.Header().Set("Content-Type", "application/json")
		if _, ok := b["parse_mode"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(entityParseBody))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	origBaseURL := TelegramBaseURL
	TelegramBaseURL = server.URL
	defer func() { TelegramBaseURL = origBaseURL }()

	body := Render(model.ChannelKindTelegram, template, map[string]interface{}{"workflow_final_result": summary})
	if utf16Len(body) <= 4096 {
		t.Fatalf("test body must exceed a single Telegram message, got %d units", utf16Len(body))
	}

	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"bot_token": "tok", "chat_id": "cid"},
		Body:   body,
	})
	if err != nil {
		t.Fatalf("Send: got error %v, want nil (malformed template must still deliver via fallback)", err)
	}

	var delivered strings.Builder
	for _, req := range requests {
		if _, ok := req["parse_mode"]; ok {
			continue // rejected attempt, not delivered
		}
		text, _ := req["text"].(string)
		delivered.WriteString(text)
	}
	if !strings.Contains(strings.ReplaceAll(delivered.String(), "\n", ""), strings.Repeat("x", 100)) {
		t.Errorf("delivered plaintext does not contain the summary's repeated content")
	}
}

func TestTelegramTransport_Acceptance_WellFormedTemplateNoFallback(t *testing.T) {
	summary := strings.Repeat("x", 5900) + "**bold**" + "*" + "`" + "_x"

	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b map[string]any
		_ = json.Unmarshal(raw, &b)
		requests = append(requests, b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	origBaseURL := TelegramBaseURL
	TelegramBaseURL = server.URL
	defer func() { TelegramBaseURL = origBaseURL }()

	body := Render(model.ChannelKindTelegram, "${summary}", map[string]interface{}{"workflow_final_result": summary})

	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"bot_token": "tok", "chat_id": "cid"},
		Body:   body,
	})
	if err != nil {
		t.Fatalf("Send (well-formed template): got error %v, want nil", err)
	}
	if len(requests) == 0 {
		t.Fatal("expected at least one request")
	}
	for i, req := range requests {
		if req["parse_mode"] != "MarkdownV2" {
			t.Errorf("request %d missing parse_mode=MarkdownV2: %v", i, req)
		}
	}
}
