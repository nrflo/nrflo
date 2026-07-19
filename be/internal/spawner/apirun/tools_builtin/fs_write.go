package tools_builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// writeFileHandler implements write_file: create a new file, or fully
// overwrite an existing one that has already been read this session.
type writeFileHandler struct{}

func (writeFileHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "write_file",
		Description: "Write content to a file inside the working directory, creating it (and any missing parent directories) or fully overwriting it. Prefer edit_file for a targeted change to part of an existing file. To overwrite a file that already exists you must have read it first with read_file in this session — this prevents blindly clobbering content you have not seen.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"path":{"type":"string","description":"File path, relative to the working directory"},
"content":{"type":"string","description":"Full file content"}
},
"required":["path","content"],
"additionalProperties":false
}`),
	}
}

func (writeFileHandler) Invoke(_ context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	abs, err := resolveFSPath(env, args.Path)
	if err != nil {
		return err.Error(), true, nil
	}

	if fi, statErr := os.Stat(abs); statErr == nil {
		if fi.IsDir() {
			return fmt.Sprintf("%s is a directory", args.Path), true, nil
		}
		if env.FS != nil && !env.FS.WasRead(abs) {
			return fmt.Sprintf("%s already exists — read it first with read_file before overwriting", args.Path), true, nil
		}
	}

	if mkErr := os.MkdirAll(filepath.Dir(abs), 0o755); mkErr != nil {
		return mkErr.Error(), true, nil
	}
	if writeErr := os.WriteFile(abs, []byte(args.Content), 0o644); writeErr != nil {
		return writeErr.Error(), true, nil
	}
	env.FS.MarkRead(abs)
	return fmt.Sprintf("wrote %s (%d bytes)", args.Path, len(args.Content)), false, nil
}
