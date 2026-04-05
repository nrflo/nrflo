package spawner

import (
	"strings"
	"testing"
)

// TestFormatMessagesForSave_NilInput verifies nil input returns empty string.
func TestFormatMessagesForSave_NilInput(t *testing.T) {
	got := formatMessagesForSave(nil, 1000)
	if got != "" {
		t.Errorf("nil input = %q, want empty string", got)
	}
}

// TestFormatMessagesForSave_EmptySlice verifies empty slice returns empty string.
func TestFormatMessagesForSave_EmptySlice(t *testing.T) {
	got := formatMessagesForSave([]string{}, 1000)
	if got != "" {
		t.Errorf("empty slice = %q, want empty string", got)
	}
}

// TestFormatMessagesForSave_SingleMessage_UnderLimit verifies a single message
// under the limit is returned as-is.
func TestFormatMessagesForSave_SingleMessage_UnderLimit(t *testing.T) {
	msg := "this is a single message"
	got := formatMessagesForSave([]string{msg}, 1000)
	if got != msg {
		t.Errorf("single message = %q, want %q", got, msg)
	}
}

// TestFormatMessagesForSave_MultipleMessages_UnderLimit verifies multiple
// messages under the limit are joined with newlines.
func TestFormatMessagesForSave_MultipleMessages_UnderLimit(t *testing.T) {
	messages := []string{"message one", "message two", "message three"}
	want := "message one\nmessage two\nmessage three"
	got := formatMessagesForSave(messages, 1000)
	if got != want {
		t.Errorf("under limit = %q, want %q", got, want)
	}
}

// TestFormatMessagesForSave_ExactBoundary verifies messages that join to exactly
// maxChars are returned without a truncation header.
func TestFormatMessagesForSave_ExactBoundary(t *testing.T) {
	// "aaa\nbbb" = 7 chars
	messages := []string{"aaa", "bbb"}
	want := "aaa\nbbb"
	got := formatMessagesForSave(messages, len(want))
	if got != want {
		t.Errorf("exact boundary = %q, want %q", got, want)
	}
}

// TestFormatMessagesForSave_ExactBoundaryMinus1 verifies that one byte over
// the limit triggers truncation.
func TestFormatMessagesForSave_ExactBoundaryMinus1(t *testing.T) {
	// "aaa\nbbb" = 7 chars; maxChars = 6 → truncation, keeps "bbb"
	messages := []string{"aaa", "bbb"}
	got := formatMessagesForSave(messages, 6)

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

// TestFormatMessagesForSave_OverLimit_KeepsTail verifies that when messages
// exceed maxChars, the last (most recent) messages are kept.
func TestFormatMessagesForSave_OverLimit_KeepsTail(t *testing.T) {
	// Each message 4 chars; joined = "msg1\nmsg2\nmsg3\nmsg4\nmsg5" = 24 chars.
	// maxChars=14: fits msg3\nmsg4\nmsg5 = 4+1+4+1+4 = 14 chars exactly.
	messages := []string{"msg1", "msg2", "msg3", "msg4", "msg5"}
	got := formatMessagesForSave(messages, 14)

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

// TestFormatMessagesForSave_OverLimit_MessageOrderPreserved verifies that
// after truncation the remaining messages are in their original order.
func TestFormatMessagesForSave_OverLimit_MessageOrderPreserved(t *testing.T) {
	// "first\nsecond\nthird" = 18 chars; maxChars=12: fits "second\nthird" = 12.
	messages := []string{"first", "second", "third"}
	got := formatMessagesForSave(messages, 12)

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

// TestFormatMessagesForSave_TruncationHeaderFormat verifies the "[truncated:
// showing last N of M messages]" header format when many messages are dropped.
func TestFormatMessagesForSave_TruncationHeaderFormat(t *testing.T) {
	// 10 messages of 5 chars each; joined = 10*5 + 9 = 59 chars.
	// maxChars=20: fits 3 messages (5+1+5+1+5=17 ≤ 20; adding 4th: 17+1+5=23 > 20).
	messages := []string{
		"aaaaa", "bbbbb", "ccccc", "ddddd", "eeeee",
		"fffff", "ggggg", "hhhhh", "iiiii", "jjjjj",
	}
	got := formatMessagesForSave(messages, 20)

	if !strings.Contains(got, "[truncated: showing last") {
		t.Errorf("expected truncation header, got %q", got)
	}
	if !strings.Contains(got, "of 10 messages]") {
		t.Errorf("expected 'of 10 messages]' in header, got %q", got)
	}
	// Last 3 messages kept.
	for _, m := range []string{"hhhhh", "iiiii", "jjjjj"} {
		if !strings.Contains(got, m) {
			t.Errorf("expected %q in output, got %q", m, got)
		}
	}
	// First 7 messages dropped.
	for _, m := range []string{"aaaaa", "bbbbb", "ccccc", "ddddd", "eeeee", "fffff", "ggggg"} {
		if strings.Contains(got, m) {
			t.Errorf("unexpected %q in output, got %q", m, got)
		}
	}
}

// TestFormatMessagesForSave_SingleMessageOverLimit verifies a single message
// that exceeds maxChars still returns a truncation header with 0 kept messages.
func TestFormatMessagesForSave_SingleMessageOverLimit(t *testing.T) {
	// "toolong" = 7 chars, maxChars = 3: cannot keep even one message.
	got := formatMessagesForSave([]string{"toolong"}, 3)

	if !strings.Contains(got, "[truncated: showing last 0 of 1 messages]") {
		t.Errorf("expected header showing 0 of 1, got %q", got)
	}
}
