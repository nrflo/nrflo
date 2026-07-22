package foldfmt

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestJoinTail_NilInput(t *testing.T) {
	t.Parallel()
	got := JoinTail(nil, 1000)
	if got != "" {
		t.Errorf("nil input = %q, want empty string", got)
	}
}

func TestJoinTail_EmptySlice(t *testing.T) {
	t.Parallel()
	got := JoinTail([]string{}, 1000)
	if got != "" {
		t.Errorf("empty slice = %q, want empty string", got)
	}
}

func TestJoinTail_SingleMessage_UnderLimit(t *testing.T) {
	t.Parallel()
	msg := "this is a single message"
	got := JoinTail([]string{msg}, 1000)
	if got != msg {
		t.Errorf("single message = %q, want %q", got, msg)
	}
}

func TestJoinTail_MultipleMessages_UnderLimit(t *testing.T) {
	t.Parallel()
	messages := []string{"message one", "message two", "message three"}
	want := "message one\nmessage two\nmessage three"
	got := JoinTail(messages, 1000)
	if got != want {
		t.Errorf("under limit = %q, want %q", got, want)
	}
}

func TestJoinTail_ExactBoundary(t *testing.T) {
	t.Parallel()
	// "aaa\nbbb" = 7 chars
	messages := []string{"aaa", "bbb"}
	want := "aaa\nbbb"
	got := JoinTail(messages, len(want))
	if got != want {
		t.Errorf("exact boundary = %q, want %q", got, want)
	}
}

func TestJoinTail_ExactBoundaryMinus1(t *testing.T) {
	t.Parallel()
	// "aaa\nbbb" = 7 chars; maxChars = 6 -> truncation, keeps "bbb"
	messages := []string{"aaa", "bbb"}
	got := JoinTail(messages, 6)

	if !strings.HasPrefix(got, "[truncated: showing last 1 of 2 messages]") {
		t.Errorf("expected truncation header, got %q", got)
	}
	if !strings.Contains(got, "bbb") {
		t.Errorf("expected 'bbb' in output, got %q", got)
	}
	if strings.Contains(got, "aaa") {
		t.Errorf("did not expect 'aaa' in output, got %q", got)
	}
}

func TestJoinTail_OverLimit_KeepsTail(t *testing.T) {
	t.Parallel()
	// Each message 4 chars; joined = "msg1\nmsg2\nmsg3\nmsg4\nmsg5" = 24 chars.
	// maxChars=14: fits msg3\nmsg4\nmsg5 = 4+1+4+1+4 = 14 chars exactly.
	messages := []string{"msg1", "msg2", "msg3", "msg4", "msg5"}
	got := JoinTail(messages, 14)

	if !strings.HasPrefix(got, "[truncated: showing last 3 of 5 messages]") {
		t.Errorf("expected truncation header, got %q", got)
	}
	for _, m := range []string{"msg3", "msg4", "msg5"} {
		if !strings.Contains(got, m) {
			t.Errorf("expected %q in truncated output, got %q", m, got)
		}
	}
	for _, m := range []string{"msg1", "msg2"} {
		if strings.Contains(got, m) {
			t.Errorf("did not expect %q in truncated output, got %q", m, got)
		}
	}
}

func TestJoinTail_OverLimit_MessageOrderPreserved(t *testing.T) {
	t.Parallel()
	// "first\nsecond\nthird" = 18 chars; maxChars=12: fits "second\nthird" = 12.
	messages := []string{"first", "second", "third"}
	got := JoinTail(messages, 12)

	expectedHeader := "[truncated: showing last 2 of 3 messages]"
	if !strings.HasPrefix(got, expectedHeader) {
		t.Errorf("expected header %q, got %q", expectedHeader, got)
	}

	secondIdx := strings.Index(got, "second")
	thirdIdx := strings.Index(got, "third")
	if secondIdx == -1 || thirdIdx == -1 {
		t.Fatalf("expected both 'second' and 'third' in output, got %q", got)
	}
	if secondIdx > thirdIdx {
		t.Errorf("'second' should appear before 'third', got %q", got)
	}
	if strings.Contains(got, "first") {
		t.Errorf("'first' should not appear in output, got %q", got)
	}
}

func TestJoinTail_TruncationHeaderFormat(t *testing.T) {
	t.Parallel()
	// 10 messages of 5 chars each; joined = 10*5 + 9 = 59 chars.
	// maxChars=20: fits 3 messages (5+1+5+1+5=17 <= 20; adding 4th: 17+1+5=23 > 20).
	messages := []string{
		"aaaaa", "bbbbb", "ccccc", "ddddd", "eeeee",
		"fffff", "ggggg", "hhhhh", "iiiii", "jjjjj",
	}
	got := JoinTail(messages, 20)

	if !strings.Contains(got, "[truncated: showing last") {
		t.Errorf("expected truncation header, got %q", got)
	}
	if !strings.Contains(got, "of 10 messages]") {
		t.Errorf("expected 'of 10 messages]' in header, got %q", got)
	}
	for _, m := range []string{"hhhhh", "iiiii", "jjjjj"} {
		if !strings.Contains(got, m) {
			t.Errorf("expected %q in output, got %q", m, got)
		}
	}
	for _, m := range []string{"aaaaa", "bbbbb", "ccccc", "ddddd", "eeeee", "fffff", "ggggg"} {
		if strings.Contains(got, m) {
			t.Errorf("unexpected %q in output, got %q", m, got)
		}
	}
}

// TestJoinTail_SingleMessageOverLimit replaces the old buggy assertion
// (header-only, "showing last 0 of 1 messages", no content) with the fixed
// behavior: the oversized message is hard-truncated and its content kept.
func TestJoinTail_SingleMessageOverLimit(t *testing.T) {
	t.Parallel()
	// "toolong" = 7 chars, maxChars = 3: cannot keep even one whole message.
	got := JoinTail([]string{"toolong"}, 3)

	if strings.Contains(got, "showing last 0 of 1") {
		t.Errorf("must not fall back to header-only zero-kept form, got %q", got)
	}
	if !strings.Contains(got, "too") {
		t.Errorf("expected truncated content 'too' in output, got %q", got)
	}
	if !strings.Contains(got, "[message truncated to 3 bytes]") {
		t.Errorf("expected truncation marker, got %q", got)
	}
}

// TestJoinTail_LargeSingleMessageOverLimit is the ticket case: a message far
// larger than maxChars must still yield real, sizeable content, not just a
// header.
func TestJoinTail_LargeSingleMessageOverLimit(t *testing.T) {
	t.Parallel()
	msg := strings.Repeat("x", 20000)
	got := JoinTail([]string{msg}, 8000)

	if strings.HasPrefix(got, "[truncated: showing last 0") {
		t.Errorf("must not be header-only, got prefix %q", got[:min(60, len(got))])
	}
	if !strings.Contains(got, strings.Repeat("x", 8000)) {
		t.Errorf("expected 8000-byte run of message content in output")
	}
	if !strings.Contains(got, "[message truncated to 8000 bytes]") {
		t.Errorf("expected truncation marker, got prefix %q", got[:min(60, len(got))])
	}
	if len(got) < 8000 {
		t.Errorf("expected output >= 8000 bytes, got %d", len(got))
	}
}

// TestJoinTail_NewestOversizedAmongMany verifies the oversized-message branch
// keys off messages[len-1]: only the truncated newest message survives, all
// earlier smaller messages are dropped.
func TestJoinTail_NewestOversizedAmongMany(t *testing.T) {
	t.Parallel()
	messages := []string{"qqq1", "qqq2", "qqq3", strings.Repeat("z", 100)}
	got := JoinTail(messages, 10)

	if strings.Contains(got, "showing last 0 of") {
		t.Errorf("must not fall back to header-only zero-kept form, got %q", got)
	}
	if !strings.Contains(got, "[message truncated to 10 bytes]") {
		t.Errorf("expected truncation marker, got %q", got)
	}
	if !strings.Contains(got, strings.Repeat("z", 10)) {
		t.Errorf("expected truncated newest-message content, got %q", got)
	}
	for _, m := range []string{"qqq1", "qqq2", "qqq3"} {
		if strings.Contains(got, m) {
			t.Errorf("did not expect earlier message %q dropped-content in output, got %q", m, got)
		}
	}
}

func TestCapBytes_UnderLimit(t *testing.T) {
	t.Parallel()
	got := CapBytes("short", 100)
	if got != "short" {
		t.Errorf("CapBytes under limit = %q, want %q", got, "short")
	}
}

// TestCapBytes_RuneBoundary verifies a cap that would split a multi-byte
// rune backs off to the nearest valid UTF-8 boundary.
func TestCapBytes_RuneBoundary(t *testing.T) {
	t.Parallel()
	s := "aé" // 'a' (1 byte) + 'é' (2 bytes, U+00E9) = 3 bytes total
	if len(s) != 3 {
		t.Fatalf("test fixture assumption broken: len(%q) = %d, want 3", s, len(s))
	}
	// Cap at 2 bytes splits the middle of 'é' (byte offset 2 is its 2nd byte).
	got := CapBytes(s, 2)

	if !utf8.ValidString(got) {
		t.Errorf("CapBytes(%q, 2) = %q is not valid UTF-8", s, got)
	}
	if got != "a" {
		t.Errorf("CapBytes(%q, 2) = %q, want %q (back off past split rune)", s, got, "a")
	}
}

func TestCapBytes_ExactBoundary(t *testing.T) {
	t.Parallel()
	got := CapBytes("hello", 5)
	if got != "hello" {
		t.Errorf("CapBytes exact boundary = %q, want %q", got, "hello")
	}
}
