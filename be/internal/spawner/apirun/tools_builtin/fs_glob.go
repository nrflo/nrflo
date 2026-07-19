package tools_builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

const globMaxResults = 500

// globHandler implements glob: fast file-pattern matching under the working
// directory, sorted by modification time.
type globHandler struct{}

func (globHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "glob",
		Description: "Fast file-pattern matching for any codebase size. Supports glob patterns like \"**/*.js\" or \"src/**/*.ts\" (** matches any number of directories). Returns matching file paths sorted by modification time, most recent first. Use this when you know part of a file name or path pattern; for open-ended content search use grep instead.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"pattern":{"type":"string","description":"Glob pattern, relative to the working directory (\"**\" matches any number of directories)"}
},
"required":["pattern"],
"additionalProperties":false
}`),
	}
}

func (globHandler) Invoke(_ context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.WorkDir == "" {
		return "no working directory configured for this session", true, nil
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return "pattern is required", true, nil
	}
	root, err := filepath.EvalSymlinks(env.WorkDir)
	if err != nil {
		return err.Error(), true, nil
	}
	re, err := globToRegexp(args.Pattern)
	if err != nil {
		return "invalid pattern: " + err.Error(), true, nil
	}

	type hit struct {
		rel   string
		mtime int64
	}
	var hits []hit
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !re.MatchString(rel) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		hits = append(hits, hit{rel: rel, mtime: info.ModTime().UnixNano()})
		return nil
	})
	if walkErr != nil {
		return walkErr.Error(), true, nil
	}

	if len(hits) == 0 {
		return "no files matched " + args.Pattern, false, nil
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].mtime > hits[j].mtime })
	truncated := len(hits) > globMaxResults
	if truncated {
		hits = hits[:globMaxResults]
	}
	var out strings.Builder
	for _, h := range hits {
		out.WriteString(h.rel)
		out.WriteString("\n")
	}
	if truncated {
		fmt.Fprintf(&out, "… truncated to first %d matches\n", globMaxResults)
	}
	return strings.TrimRight(out.String(), "\n"), false, nil
}

// globToRegexp converts a doublestar-shaped glob pattern (** matches any
// number of path segments) into an anchored regexp over forward-slash
// relative paths. Shared with grep's optional glob filter.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	pattern = filepath.ToSlash(pattern)
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 3
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i += 2
		case pattern[i] == '*':
			b.WriteString("[^/]*")
			i++
		case pattern[i] == '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
