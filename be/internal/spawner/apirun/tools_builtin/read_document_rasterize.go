package tools_builtin

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"be/internal/spawner/apirun/provider"
)

const (
	// rasterDPI is the pdftoppm render resolution. 150 keeps small print and
	// dense glyphs legible for OCR without ballooning per-page payloads.
	rasterDPI = 150
	// rasterMaxPages caps how many PDF pages are rendered into one tool result.
	rasterMaxPages = 16
	// rasterTimeout bounds the pdftoppm run.
	rasterTimeout = 60 * time.Second
)

// lookPdftoppm resolves the pdftoppm binary on the server host; a package var
// so tests can stub it. Absent binary → the hybrid handler falls back to the
// path-only PDF result.
var lookPdftoppm = func() (string, error) { return exec.LookPath("pdftoppm") }

// rasterizePDF renders the PDF at absPath to per-page PNG media blocks via
// pdftoppm. It renders up to rasterMaxPages+1 pages (the sentinel page only
// detects that the document continues past the cap) and stops accumulating
// when the base64 payload would exceed maxBytes. truncated reports whether a
// page or byte cap clipped the output.
func rasterizePDF(ctx context.Context, bin, absPath, name string, maxBytes int) (blocks []provider.MediaBlock, truncated bool, err error) {
	tmpDir, err := os.MkdirTemp("", "nrflo-raster-*")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(ctx, rasterTimeout)
	defer cancel()
	prefix := filepath.Join(tmpDir, "page")
	cmd := exec.CommandContext(ctx, bin,
		"-png", "-r", strconv.Itoa(rasterDPI),
		"-f", "1", "-l", strconv.Itoa(rasterMaxPages+1),
		absPath, prefix)
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		tail := string(out)
		if len(tail) > 512 {
			tail = tail[len(tail)-512:]
		}
		return nil, false, fmt.Errorf("pdftoppm: %w: %s", runErr, tail)
	}

	pages, err := filepath.Glob(prefix + "-*.png")
	if err != nil || len(pages) == 0 {
		return nil, false, fmt.Errorf("pdftoppm produced no pages")
	}
	// pdftoppm zero-pads page numbers to the width of the last rendered page,
	// so lexical order is page order within a single run.
	sort.Strings(pages)
	if len(pages) > rasterMaxPages {
		pages = pages[:rasterMaxPages]
		truncated = true
	}

	total := 0
	for i, p := range pages {
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil, false, readErr
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		if total+len(b64) > maxBytes {
			truncated = true
			break
		}
		total += len(b64)
		blocks = append(blocks, provider.MediaBlock{
			Kind:      "image",
			MediaType: "image/png",
			DataB64:   b64,
			Name:      fmt.Sprintf("%s (page %d)", name, i+1),
		})
	}
	if len(blocks) == 0 {
		return nil, false, fmt.Errorf("first rendered page exceeds the %d MiB media budget", maxBytes/(1024*1024))
	}
	return blocks, truncated, nil
}
