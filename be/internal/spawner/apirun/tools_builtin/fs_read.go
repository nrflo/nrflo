package tools_builtin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

const (
	readFileMaxBytes     = 256 << 10        // read cap per call
	readFileDefaultLine  = 2000             // default line limit per call
	readFileMaxLineLen   = 2000             // per-line length cap before truncation
	readFileMaxMediaSize = 32 * 1024 * 1024 // 32 MiB cap for image reads
)

// readFileHandler implements read_file: line-numbered text content (cat -n
// shape), resolved workdir-relative but not restricted to it (Claude Code
// parity — absolute paths anywhere on disk are honored), or a native image
// content block for PNG/JPEG files so the model can see them directly.
type readFileHandler struct{}

func (readFileHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "read_file",
		Description: "Read a file inside the working directory. Text files return line-numbered content (`N\\tline`, like cat -n); use offset/limit to page through large files — output is capped, so page through big files. PNG/JPEG images are returned as an image you can see directly instead of text.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"path":{"type":"string","description":"File path, relative to the working directory (absolute paths are also allowed, anywhere on disk)"},
"offset":{"type":"integer","description":"1-based line to start from (default 1)"},
"limit":{"type":"integer","description":"Max lines to return (default 2000)"}
},
"required":["path"],
"additionalProperties":false
}`),
	}
}

// Invoke is the text-only fallback (required by ToolHandler). The runner
// always prefers InvokeMedia for this handler since it implements
// apirun.MediaToolHandler, so this path only runs if that interface is
// bypassed (pattern: read_document.go:48).
func (h readFileHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	out, _, isErr, err := h.InvokeMedia(ctx, env, input)
	return out, isErr, err
}

func (readFileHandler) InvokeMedia(_ context.Context, env apirun.ToolEnv, input json.RawMessage) (string, []provider.MediaBlock, bool, error) {
	var args struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		out, isErr, ierr := invalidArgs(err)
		return out, nil, isErr, ierr
	}
	abs, err := resolveReadPath(env, args.Path)
	if err != nil {
		return err.Error(), nil, true, nil
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return err.Error(), nil, true, nil
	}
	if fi.IsDir() {
		return fmt.Sprintf("%s is a directory", args.Path), nil, true, nil
	}

	if kind, mediaType := classifyMedia("", args.Path); kind == "image" {
		if fi.Size() > readFileMaxMediaSize {
			return "image too large to read inline (max 32 MiB)", nil, true, nil
		}
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			return readErr.Error(), nil, true, nil
		}
		env.FS.MarkRead(abs)
		return fmt.Sprintf("Loaded %s (%s).", args.Path, mediaType),
			[]provider.MediaBlock{{
				Kind:      kind,
				MediaType: mediaType,
				DataB64:   base64.StdEncoding.EncodeToString(data),
				Name:      args.Path,
			}}, false, nil
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), nil, true, nil
	}
	lines := strings.Split(string(data), "\n")

	offset := args.Offset
	if offset < 1 {
		offset = 1
	}
	limit := args.Limit
	if limit <= 0 {
		limit = readFileDefaultLine
	}
	if offset > len(lines) {
		return fmt.Sprintf("offset %d past end of file (%d lines)", offset, len(lines)), nil, true, nil
	}

	var out strings.Builder
	written := 0
	truncated := false
	for i := offset - 1; i < len(lines) && written < limit; i++ {
		line := lines[i]
		if len(line) > readFileMaxLineLen {
			line = line[:readFileMaxLineLen] + "… (line truncated)"
		}
		formatted := fmt.Sprintf("%6d\t%s\n", i+1, line)
		if out.Len()+len(formatted) > readFileMaxBytes {
			truncated = true
			break
		}
		out.WriteString(formatted)
		written++
	}
	if truncated || offset-1+written < len(lines) {
		fmt.Fprintf(&out, "… truncated (%d of %d lines shown; continue with offset=%d)\n",
			written, len(lines), offset+written)
	}
	env.FS.MarkRead(abs)
	return out.String(), nil, false, nil
}
