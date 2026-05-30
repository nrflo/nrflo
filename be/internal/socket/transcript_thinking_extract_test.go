package socket

import (
	"testing"
)

// TestExtractThinking covers all extractThinking parsing cases via a table.
func TestExtractThinking(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "one thinking block",
			line: `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"hello"}]}}`,
			want: []string{"hello"},
		},
		{
			name: "multiple thinking blocks",
			line: `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"first"},{"type":"text","text":"ignored"},{"type":"thinking","thinking":"second"}]}}`,
			want: []string{"first", "second"},
		},
		{
			name: "user type → nil",
			line: `{"type":"user","message":{"content":[{"type":"thinking","thinking":"hidden"}]}}`,
			want: nil,
		},
		{
			name: "result type → nil",
			line: `{"type":"result","message":{"content":[{"type":"thinking","thinking":"hidden"}]}}`,
			want: nil,
		},
		{
			name: "system type → nil",
			line: `{"type":"system","message":{"content":[{"type":"thinking","thinking":"hidden"}]}}`,
			want: nil,
		},
		{
			name: "redacted_thinking → skipped",
			line: `{"type":"assistant","message":{"content":[{"type":"redacted_thinking","data":"opaque"}]}}`,
			want: nil,
		},
		{
			name: "element missing thinking key → skipped",
			line: `{"type":"assistant","message":{"content":[{"type":"thinking"}]}}`,
			want: nil,
		},
		{
			name: "empty thinking text → skipped",
			line: `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":""}]}}`,
			want: nil,
		},
		{
			name: "non-JSON line → nil",
			line: `not valid json at all`,
			want: nil,
		},
		{
			name: "empty line → nil",
			line: ``,
			want: nil,
		},
		{
			name: "no message key → nil",
			line: `{"type":"assistant"}`,
			want: nil,
		},
		{
			name: "content not array → nil",
			line: `{"type":"assistant","message":{"content":"scalar"}}`,
			want: nil,
		},
		{
			name: "text element mixed in → only thinking returned",
			line: `{"type":"assistant","message":{"content":[{"type":"text","text":"prose"},{"type":"thinking","thinking":"thought"},{"type":"tool_use","name":"Bash"}]}}`,
			want: []string{"thought"},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := extractThinking([]byte(c.line))
			if len(got) != len(c.want) {
				t.Fatalf("extractThinking(%s) = %v (len %d), want len %d", c.name, got, len(got), len(c.want))
			}
			for i, w := range c.want {
				if got[i] != w {
					t.Errorf("extractThinking[%d] = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}
