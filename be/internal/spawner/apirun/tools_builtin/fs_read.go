package tools_builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

const (
	readFileMaxBytes    = 256 << 10 // read cap per call
	readFileDefaultLine = 2000      // default line limit per call
)

// readFileHandler implements read_file: line-numbered file content from
// inside the session workdir jail.
type readFileHandler struct{}

func (readFileHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "read_file",
		Description: "Read a text file inside the working directory. Returns line-numbered content (`N\\tline`). Use offset/limit for large files; output is capped, so page through big files.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"path":{"type":"string","description":"File path, relative to the working directory (absolute allowed if inside it)"},
"offset":{"type":"integer","description":"1-based line to start from (default 1)"},
"limit":{"type":"integer","description":"Max lines to return (default 2000)"}
},
"required":["path"],
"additionalProperties":false
}`),
	}
}

func (readFileHandler) Invoke(_ context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	abs, err := resolveFSPath(env, args.Path)
	if err != nil {
		return err.Error(), true, nil
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return err.Error(), true, nil
	}
	if fi.IsDir() {
		return fmt.Sprintf("%s is a directory", args.Path), true, nil
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true, nil
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
		return fmt.Sprintf("offset %d past end of file (%d lines)", offset, len(lines)), true, nil
	}

	var out strings.Builder
	written := 0
	truncated := false
	for i := offset - 1; i < len(lines) && written < limit; i++ {
		line := fmt.Sprintf("%6d\t%s\n", i+1, lines[i])
		if out.Len()+len(line) > readFileMaxBytes {
			truncated = true
			break
		}
		out.WriteString(line)
		written++
	}
	if truncated || offset-1+written < len(lines) {
		fmt.Fprintf(&out, "… truncated (%d of %d lines shown; continue with offset=%d)\n",
			written, len(lines), offset+written)
	}
	return out.String(), false, nil
}
