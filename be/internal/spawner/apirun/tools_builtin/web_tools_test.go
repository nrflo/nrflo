package tools_builtin

import (
	"strings"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"https://www.Example.com/Path/": "example.com/path",
		"https://example.com/path":      "example.com/path",
		"https://EXAMPLE.com":           "example.com",
		"http://www.example.com/a/":     "example.com/a",
	}
	for in, want := range cases {
		if got := normalizeURL(in); got != want {
			t.Errorf("normalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDedupeAndCap(t *testing.T) {
	rows := []searchRow{
		{Query: "q1", URL: "https://a.com/x"},
		{Query: "q2", URL: "https://www.a.com/x/"}, // dup of first (normalized)
		{Query: "q1", URL: "https://a.com/y"},
		{Query: "q1", URL: "https://a.com/z"}, // 3rd a.com -> over cap=2
		{Query: "q1", URL: "https://b.com/1"},
	}
	out := dedupeAndCap(rows, 2)
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3 (dup dropped, a.com capped at 2, b.com kept)", len(out))
	}
	// a.com appears at most twice
	count := 0
	for _, r := range out {
		if hostOf(r.URL) == "a.com" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("a.com count = %d, want 2", count)
	}
}

func TestDedupeAndCap_NoCap(t *testing.T) {
	rows := []searchRow{
		{URL: "https://a.com/1"}, {URL: "https://a.com/2"}, {URL: "https://a.com/3"},
	}
	if out := dedupeAndCap(rows, 0); len(out) != 3 {
		t.Errorf("len = %d, want 3 when perDomain=0 (no cap)", len(out))
	}
}

func TestClip(t *testing.T) {
	// Whole string when within bounds: no truncation, no marker.
	if got, trunc := clip("short", 100); got != "short" || trunc {
		t.Errorf("clip(short,100) = %q,%v want \"short\",false", got, trunc)
	}
	// Over the threshold: body is exactly the first n bytes, marker is NOT
	// baked in (the handler adds it). This is the fix for the offload bug —
	// truncation is signaled by the bool, not by comparing marked lengths.
	long := strings.Repeat("a", 50)
	got, trunc := clip(long, 10)
	if !trunc {
		t.Fatal("clip(long,10) truncated=false, want true")
	}
	if got != strings.Repeat("a", 10) {
		t.Errorf("clip body = %q, want 10 'a's with no marker", got)
	}
}

func TestClip_RuneBoundary(t *testing.T) {
	// 5 multibyte runes (3 bytes each = 15 bytes); cut at 10 must not split a rune.
	got, trunc := clip(strings.Repeat("世", 5), 10)
	if !trunc {
		t.Fatal("expected truncation")
	}
	if len(got)%3 != 0 {
		t.Errorf("clip split a multibyte rune: %d bytes", len(got))
	}
}

func TestDedupeStrings(t *testing.T) {
	out := dedupeStrings([]string{"a", "b", "a", "c", "b"})
	if len(out) != 3 || out[0] != "a" || out[1] != "b" || out[2] != "c" {
		t.Errorf("dedupeStrings = %v, want [a b c]", out)
	}
}
