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

const editFileMaxBytes = 4 << 20 // refuse to edit files larger than this

// editFileHandler implements edit_file: exact-string replacement (the
// Claude-Code-shaped Edit contract) against an existing file that has already
// been read this session. Creating a new file is write_file's job.
type editFileHandler struct{}

func (editFileHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "edit_file",
		Description: "Edit an existing file inside the working directory by exact string replacement. You must read_file the file in this session before editing it. old_string must match exactly once in the file (use replace_all to replace every occurrence) and must differ from new_string. Use write_file to create a new file.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"path":{"type":"string","description":"File path, relative to the working directory"},
"old_string":{"type":"string","description":"Exact text to replace; must match uniquely unless replace_all is set"},
"new_string":{"type":"string","description":"Replacement text"},
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
	if args.OldString == args.NewString {
		return "old_string and new_string are identical — nothing to change", true, nil
	}
	abs, err := resolveFSPath(env, args.Path)
	if err != nil {
		return err.Error(), true, nil
	}

	fi, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("%s does not exist — use write_file to create it", args.Path), true, nil
		}
		return err.Error(), true, nil
	}
	if fi.IsDir() {
		return fmt.Sprintf("%s is a directory", args.Path), true, nil
	}
	if env.FS != nil && !env.FS.WasRead(abs) {
		return fmt.Sprintf("%s has not been read in this session — read_file it first before editing", args.Path), true, nil
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
	env.FS.MarkRead(abs)
	return fmt.Sprintf("edited %s (%d replacement(s))", args.Path, replaced), false, nil
}
