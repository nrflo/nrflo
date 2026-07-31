package tools_builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestJPEG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{0, 255, 0, 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func invokeMedia(t *testing.T, name string, env apirun.ToolEnv, args string) (string, []provider.MediaBlock, bool) {
	t.Helper()
	h, ok := FSTools()[name]
	if !ok {
		t.Fatalf("no fs tool %q", name)
	}
	mh, ok := h.(apirun.MediaToolHandler)
	if !ok {
		t.Fatalf("%s does not implement MediaToolHandler", name)
	}
	out, media, isErr, err := mh.InvokeMedia(context.Background(), env, json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s InvokeMedia returned Go error: %v", name, err)
	}
	return out, media, isErr
}

func TestFSTools_Read_PNGReturnsImageMediaBlock(t *testing.T) {
	env := fsEnv(t)
	writeTestPNG(t, filepath.Join(env.WorkDir, "pic.png"))

	out, media, isErr := invokeMedia(t, "read_file", env, `{"path":"pic.png"}`)
	if isErr {
		t.Fatalf("read_file png = (%q, %v)", out, isErr)
	}
	if len(media) != 1 {
		t.Fatalf("media blocks = %d, want 1", len(media))
	}
	if media[0].Kind != "image" || media[0].MediaType != "image/png" {
		t.Errorf("media block = %+v, want kind=image mediaType=image/png", media[0])
	}
	if media[0].DataB64 == "" {
		t.Error("media block DataB64 is empty")
	}
}

func TestFSTools_Read_JPEGReturnsImageMediaBlock(t *testing.T) {
	env := fsEnv(t)
	writeTestJPEG(t, filepath.Join(env.WorkDir, "pic.jpg"))

	out, media, isErr := invokeMedia(t, "read_file", env, `{"path":"pic.jpg"}`)
	if isErr {
		t.Fatalf("read_file jpeg = (%q, %v)", out, isErr)
	}
	if len(media) != 1 || media[0].Kind != "image" || media[0].MediaType != "image/jpeg" {
		t.Errorf("media = %+v, want single image/jpeg block", media)
	}
}

func TestFSTools_Read_ImageMarksReadSet(t *testing.T) {
	env := fsEnv(t)
	writeTestPNG(t, filepath.Join(env.WorkDir, "pic.png"))
	if out, _, isErr := invokeMedia(t, "read_file", env, `{"path":"pic.png"}`); isErr {
		t.Fatalf("read_file png = (%q, %v)", out, isErr)
	}
	// Reading an image marks the path read, so a subsequent write_file
	// overwrite of the same path is allowed without a separate read.
	out, isErr := invokeFS(t, "write_file", env, `{"path":"pic.png","content":"clobbered"}`)
	if isErr {
		t.Errorf("write_file after image read = (%q, %v), want success", out, isErr)
	}
}

func TestFSTools_Read_LongLineTruncation(t *testing.T) {
	env := fsEnv(t)
	longLine := strings.Repeat("x", 5000)
	if err := os.WriteFile(filepath.Join(env.WorkDir, "long.txt"), []byte(longLine+"\nshort\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFS(t, "read_file", env, `{"path":"long.txt"}`)
	if isErr {
		t.Fatalf("read_file long line = (%q, %v)", out, isErr)
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("read_file long line output missing truncation marker: %q", out[:200])
	}
	if strings.Contains(out, strings.Repeat("x", 3000)) {
		t.Errorf("read_file did not cap the long line at 2000 chars")
	}
}

func TestFSTools_Read_MarksReadSetForEdit(t *testing.T) {
	env := fsEnv(t)
	if err := os.WriteFile(filepath.Join(env.WorkDir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, isErr := invokeFS(t, "read_file", env, `{"path":"a.txt"}`); isErr {
		t.Fatalf("read_file = (%q, %v)", out, isErr)
	}
	out, isErr := invokeFS(t, "edit_file", env, `{"path":"a.txt","old_string":"hello","new_string":"bye"}`)
	if isErr {
		t.Errorf("edit_file after read_file = (%q, %v), want success", out, isErr)
	}
}

func TestFSTools_Read_OffsetPastEnd(t *testing.T) {
	env := fsEnv(t)
	if err := os.WriteFile(filepath.Join(env.WorkDir, "a.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFS(t, "read_file", env, `{"path":"a.txt","offset":100}`)
	if !isErr || !strings.Contains(out, "past end of file") {
		t.Errorf("read_file offset past end = (%q, %v), want past-end error", out, isErr)
	}
}

func TestFSTools_Read_Directory(t *testing.T) {
	env := fsEnv(t)
	if err := os.Mkdir(filepath.Join(env.WorkDir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFS(t, "read_file", env, `{"path":"adir"}`)
	if !isErr || !strings.Contains(out, "is a directory") {
		t.Errorf("read_file on directory = (%q, %v), want directory error", out, isErr)
	}
}

// TestFSTools_Read_OutsideWorkdirAbsolutePath asserts the motivating parity
// case: an absolute path outside the workdir reads successfully instead of
// refusing with an "escapes" error (Claude Code's own Read has no cwd jail).
func TestFSTools_Read_OutsideWorkdirAbsolutePath(t *testing.T) {
	env := fsEnv(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "foreign.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFS(t, "read_file", env, fmt.Sprintf(`{"path":%q}`, filepath.Join(outside, "foreign.txt")))
	if isErr {
		t.Fatalf("read_file outside workdir = (%q, %v), want success", out, isErr)
	}
	if !strings.Contains(out, "1\tone") || !strings.Contains(out, "2\ttwo") {
		t.Errorf("read_file outside workdir = %q, want line-numbered content", out)
	}
}

// TestFSTools_Read_InTreeSymlinkToOutsideDir mirrors the above through a
// symlink planted inside the workdir pointing at the foreign directory.
func TestFSTools_Read_InTreeSymlinkToOutsideDir(t *testing.T) {
	env := fsEnv(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "foreign.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(env.WorkDir, "link")); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFS(t, "read_file", env, `{"path":"link/foreign.txt"}`)
	if isErr {
		t.Fatalf("read_file via in-tree symlink = (%q, %v), want success", out, isErr)
	}
	if !strings.Contains(out, "1\thello") {
		t.Errorf("read_file via in-tree symlink = %q, want line-numbered content", out)
	}
}

// TestFSTools_Read_RelativePathStillResolvesUnderWorkdir asserts the
// unjailed read path did not break the ordinary workdir-relative case.
func TestFSTools_Read_RelativePathStillResolvesUnderWorkdir(t *testing.T) {
	env := fsEnv(t)
	if err := os.WriteFile(filepath.Join(env.WorkDir, "in.txt"), []byte("relative\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, isErr := invokeFS(t, "read_file", env, `{"path":"in.txt"}`)
	if isErr || !strings.Contains(out, "1\trelative") {
		t.Errorf("read_file relative path = (%q, %v), want line-numbered content", out, isErr)
	}
}

// TestFSTools_Read_OutsideWorkdirPNGReturnsMediaBlock is the motivating
// screenshot case: a PNG outside the workdir still returns an image block.
func TestFSTools_Read_OutsideWorkdirPNGReturnsMediaBlock(t *testing.T) {
	env := fsEnv(t)
	outside := t.TempDir()
	picPath := filepath.Join(outside, "screenshot.png")
	writeTestPNG(t, picPath)

	out, media, isErr := invokeMedia(t, "read_file", env, fmt.Sprintf(`{"path":%q}`, picPath))
	if isErr {
		t.Fatalf("read_file outside png = (%q, %v)", out, isErr)
	}
	if len(media) != 1 || media[0].Kind != "image" || media[0].MediaType != "image/png" {
		t.Errorf("media = %+v, want single image/png block", media)
	}
}

// TestFSTools_Read_EditOutsideWorkdirStillRefused proves the read/write
// split: reading an out-of-tree file succeeds, but edit_file on the same
// path is still refused with "escapes" — edits stay jailed.
func TestFSTools_Read_EditOutsideWorkdirStillRefused(t *testing.T) {
	env := fsEnv(t)
	outside := t.TempDir()
	foreign := filepath.Join(outside, "foreign.txt")
	if err := os.WriteFile(foreign, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, isErr := invokeFS(t, "read_file", env, fmt.Sprintf(`{"path":%q}`, foreign)); isErr {
		t.Fatalf("read_file outside workdir = (%q, %v), want success", out, isErr)
	}
	out, isErr := invokeFS(t, "edit_file", env, fmt.Sprintf(`{"path":%q,"old_string":"hello","new_string":"bye"}`, foreign))
	if !isErr || !strings.Contains(out, "escapes") {
		t.Errorf("edit_file outside workdir after read = (%q, %v), want escape error", out, isErr)
	}
}

// TestFSTools_Read_ImageTooLargeGuard exercises the 32 MiB inline-media cap
// without actually allocating 32 MiB: it fakes a large PNG-suffixed file and
// relies on the size check (via os.Stat) short-circuiting before any read.
func TestFSTools_Read_ImageTooLargeGuard(t *testing.T) {
	env := fsEnv(t)
	path := filepath.Join(env.WorkDir, "huge.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(readFileMaxMediaSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	out, media, isErr := invokeMedia(t, "read_file", env, fmt.Sprintf(`{"path":%q}`, "huge.png"))
	if !isErr || len(media) != 0 || !strings.Contains(out, "too large") {
		t.Errorf("read_file oversized image = (%q, %v, media=%v), want too-large error", out, isErr, media)
	}
}
