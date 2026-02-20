package usagelimits

import (
	"strings"
	"testing"
)

func TestStripANSI_Empty(t *testing.T) {
	got := stripANSI([]byte{})
	if got != "" {
		t.Errorf("stripANSI(empty) = %q, want empty", got)
	}
}

func TestStripANSI_PlainText(t *testing.T) {
	input := "hello world 123"
	got := stripANSI([]byte(input))
	if got != input {
		t.Errorf("stripANSI(%q) = %q, want unchanged", input, got)
	}
}

func TestStripANSI_CursorRight(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "5 spaces",
			input: "Rese\x1b[5Cs",
			want:  "Rese     s",
		},
		{
			name:  "1 space",
			input: "a\x1b[1Cb",
			want:  "a b",
		},
		{
			name:  "zero clamped to 1",
			input: "a\x1b[0Cb",
			want:  "a b",
		},
		{
			name:  "200 spaces at max",
			input: "\x1b[200C",
			want:  strings.Repeat(" ", 200),
		},
		{
			name:  "201 clamped to 200",
			input: "\x1b[201C",
			want:  strings.Repeat(" ", 200),
		},
		{
			name:  "multiple cursor-right",
			input: "a\x1b[2Cb\x1b[3Cc",
			want:  "a  b   c",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI([]byte(tt.input))
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripANSI_SGR(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"reset code", "\x1b[0mhello", "hello"},
		{"red foreground", "\x1b[31mtext\x1b[0m", "text"},
		{"bold green", "\x1b[1;32mtext\x1b[0m", "text"},
		{"multi-digit params", "\x1b[38;5;200mcolor\x1b[0m", "color"},
		{"cursor movement up", "\x1b[2Atext", "text"},
		{"cursor movement down", "\x1b[3Btext", "text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI([]byte(tt.input))
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripANSI_OSC(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"title sequence", "\x1b]0;window title\x07text", "text"},
		{"empty OSC body", "\x1b]0;\x07text", "text"},
		{"OSC before and after text", "before\x1b]0;title\x07after", "beforeafter"},
		{"multiple OSC", "\x1b]0;a\x07x\x1b]0;b\x07y", "xy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI([]byte(tt.input))
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripANSI_Charset(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ESC(B designator", "\x1b(Btext", "text"},
		{"ESC)0 designator", "\x1b)0text", "text"},
		{"ESC(A designator", "\x1b(Atext", "text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI([]byte(tt.input))
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripANSI_ControlChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"SI byte stripped", "text\x0fmore", "textmore"},
		{"CRLF becomes LF", "line1\r\nline2", "line1\nline2"},
		{"CR only stripped", "line\r", "line"},
		{"multiple CRs", "a\rb\rc", "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI([]byte(tt.input))
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripANSI_DeviceMode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"cursor hide", "\x1b[?25ltext", "text"},
		{"cursor show", "\x1b[?25htext", "text"},
		{"bracketed paste on", "\x1b[?2004htext", "text"},
		{"bracketed paste off", "\x1b[?2004ltext", "text"},
		{"multi-param device mode", "\x1b[?1;2htext", "text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI([]byte(tt.input))
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripANSI_Mixed(t *testing.T) {
	// Realistic terminal output with multiple escape types:
	// device mode + OSC + SGR + cursor-right (producing "Rese     s") + CR
	input := "\x1b[?25l\x1b]0;claude\x07\x1b[1;32mCurrent session\x1b[0m\r\n  Rese\x1b[4Cs 45.2% used\r\n\x1b[?25h"
	got := stripANSI([]byte(input))

	if strings.Contains(got, "\x1b") {
		t.Errorf("result still contains ESC sequences: %q", got)
	}
	if strings.Contains(got, "\r") {
		t.Errorf("result still contains CR: %q", got)
	}
	// cursor-right-4 should expand to 4 spaces: "Rese    s"
	if !strings.Contains(got, "Rese    s") {
		t.Errorf("cursor-right-4 not expanded to 4 spaces, got: %q", got)
	}
	// "Current session" text should survive
	if !strings.Contains(got, "Current session") {
		t.Errorf("text content lost, got: %q", got)
	}
}

func TestStripANSI_NoEscapeInOutput(t *testing.T) {
	// Any input with escape sequences should produce output with no ESC bytes.
	inputs := []string{
		"\x1b[31m\x1b[1mhello\x1b[0m",
		"\x1b]0;title\x07\x1b[?25l\x1b[?25h",
		"\x1b(B\x1b)0\x1b[5C",
		"\x1b[?2004h\x1b[0m\x1b[31m",
	}
	for _, input := range inputs {
		got := stripANSI([]byte(input))
		if strings.ContainsRune(got, '\x1b') {
			t.Errorf("stripANSI(%q) still contains ESC, got: %q", input, got)
		}
	}
}
