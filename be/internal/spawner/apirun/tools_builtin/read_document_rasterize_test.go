package tools_builtin

// Rasterization tests for ReadDocumentHybridHandler. pdftoppm is stubbed with
// a shell script (no poppler dependency in tests): it writes fake page PNGs
// named like pdftoppm output, keyed off an env page count.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubPdftoppm points lookPdftoppm at a script that emits `pages` fake PNG
// files (respecting the -l last-page flag like the real binary).
func stubPdftoppm(t *testing.T, pages int, exitCode int) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "pdftoppm")
	body := fmt.Sprintf(`#!/bin/sh
last=""
for a in "$@"; do prev="$last"; last="$a"; done
# $prev is the input PDF, $last the output prefix; honor -l (max last page)
limit=%d
i=1
seen_l=""
for a in "$@"; do
  if [ "$seen_l" = "1" ]; then [ "$a" -lt "$limit" ] && limit="$a"; seen_l=""; fi
  [ "$a" = "-l" ] && seen_l="1"
done
while [ "$i" -le "$limit" ]; do
  printf 'fake-png-page-%%d' "$i" > "$last-$(printf '%%02d' "$i").png"
  i=$((i+1))
done
exit %d
`, pages, exitCode)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := lookPdftoppm
	lookPdftoppm = func() (string, error) { return script, nil }
	t.Cleanup(func() { lookPdftoppm = orig })
}

func stubPdftoppmMissing(t *testing.T) {
	t.Helper()
	orig := lookPdftoppm
	lookPdftoppm = func() (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPdftoppm = orig })
}

func invokeHybridPDF(t *testing.T) (string, []int, bool) {
	t.Helper()
	env := newBuiltinTestEnv(t)
	addHybridArtifact(t, env, "deed.pdf", "%PDF-1.4 fake", "application/pdf")
	out, media, isErr, err := ReadDocumentHybridHandler{}.InvokeMedia(
		context.Background(), env.env, json.RawMessage(`{"name":"deed.pdf"}`))
	if err != nil {
		t.Fatalf("InvokeMedia err: %v", err)
	}
	sizes := make([]int, 0, len(media))
	for _, m := range media {
		if m.Kind != "image" || m.MediaType != "image/png" {
			t.Errorf("unexpected media block: %+v", m)
		}
		sizes = append(sizes, len(m.DataB64))
	}
	return out, sizes, isErr
}

func TestReadDocumentHybridHandler_PDFRasterized(t *testing.T) {
	stubPdftoppm(t, 3, 0)
	out, pages, isErr := invokeHybridPDF(t)
	if isErr {
		t.Fatalf("isErr=true, out=%q", out)
	}
	if len(pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(pages))
	}
	if !strings.Contains(out, "3 page(s) rendered") {
		t.Errorf("out = %q; want rendered-pages message", out)
	}
	if strings.Contains(out, "truncated") {
		t.Errorf("out = %q; unexpected truncation note", out)
	}
}

func TestReadDocumentHybridHandler_PDFRasterized_PageCap(t *testing.T) {
	stubPdftoppm(t, rasterMaxPages+5, 0)
	out, pages, isErr := invokeHybridPDF(t)
	if isErr {
		t.Fatalf("isErr=true, out=%q", out)
	}
	if len(pages) != rasterMaxPages {
		t.Fatalf("pages = %d, want cap %d", len(pages), rasterMaxPages)
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("out = %q; want truncation note", out)
	}
}

func TestReadDocumentHybridHandler_PDFRasterizeFails_PathFallback(t *testing.T) {
	stubPdftoppm(t, 0, 1)
	out, pages, isErr := invokeHybridPDF(t)
	if isErr {
		t.Fatalf("isErr=true, out=%q", out)
	}
	if len(pages) != 0 {
		t.Fatalf("pages = %d, want 0 on fallback", len(pages))
	}
	var result struct {
		Path string `json:"path"`
	}
	if jsonErr := json.Unmarshal([]byte(out), &result); jsonErr != nil || result.Path == "" {
		t.Errorf("want path-only fallback, got %q (err=%v)", out, jsonErr)
	}
}
