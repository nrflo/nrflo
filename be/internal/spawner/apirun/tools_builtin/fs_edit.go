package tools_builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

const editFileMaxBytes = 4 << 20 // refuse to edit files larger than this

// editFileHandler implements edit_file: exact-string replacement (the
// Claude-Code-shaped Edit contract) plus file creation when old_string is
// empty and the file does not exist.
type editFileHandler struct{}

func (editFileHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "edit_file",
		Description: "Edit a file inside the working directory by exact string replacement. old_string must match exactly once (use replace_all for every occurrence). An empty old_string creates the file with new_string (errors if it already exists). Parent directories are created as needed.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"path":{"type":"string","description":"File path, relative to the working directory"},
"old_string":{"type":"string","description":"Exact text to replace; empty to create a new file"},
"new_string":{"type":"string","description":"Replacement text (or full content for a new file)"},
"replace_all":{"type":"boolean","description":"Replace every occurrence instead of requiring a unique match"}
},
"required":["path","old_string","new_string"],
"additionalProperties":false
}`),
	}
}

func (editFileHandler) Invoke(_ context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	abs, err := resolveFSPath(env, args.Path)
	if err != nil {
		return err.Error(), true, nil
	}

	if args.OldString == "" {
		if _, statErr := os.Stat(abs); statErr == nil {
			return fmt.Sprintf("%s already exists — pass the exact old_string to edit it", args.Path), true, nil
		}
		if mkErr := os.MkdirAll(filepath.Dir(abs), 0o755); mkErr != nil {
			return mkErr.Error(), true, nil
		}
		if writeErr := os.WriteFile(abs, []byte(args.NewString), 0o644); writeErr != nil {
			return writeErr.Error(), true, nil
		}
		return fmt.Sprintf("created %s (%d bytes)", args.Path, len(args.NewString)), false, nil
	}

	fi, err := os.Stat(abs)
	if err != nil {
		return err.Error(), true, nil
	}
	if fi.Size() > editFileMaxBytes {
		return fmt.Sprintf("%s is too large to edit (%d bytes)", args.Path, fi.Size()), true, nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true, nil
	}
	content := string(data)

	count := strings.Count(content, args.OldString)
	switch {
	case count == 0:
		return "old_string not found in " + args.Path, true, nil
	case count > 1 && !args.ReplaceAll:
		return fmt.Sprintf("old_string matches %d times in %s — make it unique or set replace_all", count, args.Path), true, nil
	}

	replaced := count
	if args.ReplaceAll {
		content = strings.ReplaceAll(content, args.OldString, args.NewString)
	} else {
		content = strings.Replace(content, args.OldString, args.NewString, 1)
		replaced = 1
	}
	if err := os.WriteFile(abs, []byte(content), fi.Mode().Perm()); err != nil {
		return err.Error(), true, nil
	}
	return fmt.Sprintf("edited %s (%d replacement(s))", args.Path, replaced), false, nil
}
