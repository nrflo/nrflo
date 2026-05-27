package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSplitTelegram_ShortBodySingleChunk(t *testing.T) {
	body := "hello\nworld"
	chunks := splitTelegram(body)
	if len(chunks) != 1 || chunks[0] != body {
		t.Fatalf("short body: got %d chunks %q, want 1 chunk unchanged", len(chunks), chunks)
	}
}

func TestSplitTelegram_SplitsOnLineBoundaries(t *testing.T) {
	// 5 lines of 1000 runes each (+newlines) = ~5004 > 4096, must split.
	line := strings.Repeat("a", 1000)
	lines := make([]string, 5)
	for i := range lines {
		lines[i] = line
	}
	body := strings.Join(lines, "\n")

	chunks := splitTelegram(body)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if utf16Len(c) > telegramMaxLen {
			t.Errorf("chunk %d len %d exceeds limit %d", i, utf16Len(c), telegramMaxLen)
		}
	}
	// Concatenating chunks with the newline they were split on reconstructs body.
	if strings.Join(chunks, "\n") != body {
		t.Errorf("reconstructed body differs from original")
	}
}

func TestSplitTelegram_HardSplitsLongLine(t *testing.T) {
	body := strings.Repeat("x", 10000) // single line, no newlines
	chunks := splitTelegram(body)
	if len(chunks) < 3 {
		t.Fatalf("expected >=3 chunks for 10k line, got %d", len(chunks))
	}
	total := 0
	for i, c := range chunks {
		if utf16Len(c) > telegramMaxLen {
			t.Errorf("chunk %d len %d exceeds limit", i, utf16Len(c))
		}
		total += len(c)
	}
	if strings.Join(chunks, "") != body {
		t.Errorf("hard-split lost content")
	}
}

func TestSplitTelegram_DoesNotBreakEscapePair(t *testing.T) {
	// A line whose limit boundary lands exactly on a backslash escape pair.
	// Build "...\." such that the cut would otherwise separate '\' from '.'.
	head := strings.Repeat("a", telegramMaxLen-1)
	body := head + `\.` + strings.Repeat("b", 10)
	chunks := hardSplitLine(body, telegramMaxLen)
	if len(chunks) < 2 {
		t.Fatalf("expected hard split, got %d chunks", len(chunks))
	}
	// The first chunk must not end with a dangling odd backslash run.
	first := chunks[0]
	bs := 0
	for i := len(first) - 1; i >= 0 && first[i] == '\\'; i-- {
		bs++
	}
	if bs%2 == 1 {
		t.Errorf("first chunk ends with dangling backslash escape: %q", first[len(first)-3:])
	}
	if strings.Join(chunks, "") != body {
		t.Errorf("escape-aware split lost content")
	}
}

func TestTelegramTransport_SplitsIntoMultipleSends(t *testing.T) {
	var texts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b map[string]string
		_ = json.Unmarshal(raw, &b)
		texts = append(texts, b["text"])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	origBaseURL := TelegramBaseURL
	TelegramBaseURL = server.URL
	defer func() { TelegramBaseURL = origBaseURL }()

	body := strings.Repeat("a", 1000) + "\n" + strings.Repeat("b", 1000) + "\n" +
		strings.Repeat("c", 1000) + "\n" + strings.Repeat("d", 1000) + "\n" +
		strings.Repeat("e", 1000)

	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"bot_token": "tok", "chat_id": "cid"},
		Body:   body,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(texts) < 2 {
		t.Fatalf("expected multiple sendMessage calls, got %d", len(texts))
	}
	for i, txt := range texts {
		if utf16Len(txt) > telegramMaxLen {
			t.Errorf("message %d exceeds Telegram limit: %d", i, utf16Len(txt))
		}
	}
}

func TestTelegramTransport_PartialFailureStops(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":false,"description":"flood control"}`))
	}))
	defer server.Close()

	origBaseURL := TelegramBaseURL
	TelegramBaseURL = server.URL
	defer func() { TelegramBaseURL = origBaseURL }()

	body := strings.Repeat("a", 3000) + "\n" + strings.Repeat("b", 3000)
	tr := Get("telegram")
	err := tr.Send(&Notification{
		Config: map[string]interface{}{"bot_token": "tok", "chat_id": "cid"},
		Body:   body,
	})
	if err == nil {
		t.Fatal("expected error when a chunk fails")
	}
	if calls != 2 {
		t.Errorf("expected to stop after the failing chunk (2 calls), got %d", calls)
	}
}
