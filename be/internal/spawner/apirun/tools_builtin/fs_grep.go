package tools_builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

const (
	grepMaxFileBytes = 4 << 20  // skip files larger than this
	grepOutputCap    = 64 << 10 // hard output cap; quarantine handles anything larger downstream
)

// grepHandler implements grep: regex content search under the working
// directory with files_with_matches/count/content output modes.
type grepHandler struct{}

func (grepHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "grep",
		Description: "Search file contents with a regular expression, powered by ripgrep semantics. Supports full regex syntax (e.g. \"log.*Error\", \"function\\\\s+\\\\w+\"). Filter which files are searched with glob (e.g. \"*.go\"). output_mode: \"files_with_matches\" (default) lists matching file paths, \"count\" shows per-file match counts, \"content\" shows matching lines with line numbers and optional -A/-B/-C context. Use this instead of grep/find via bash.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"pattern":{"type":"string","description":"Regular expression to search for"},
"glob":{"type":"string","description":"Glob to filter which files are searched, e.g. \"*.go\" or \"**/*.ts\""},
"output_mode":{"type":"string","enum":["content","files_with_matches","count"],"description":"Defaults to files_with_matches"},
"-i":{"type":"boolean","description":"Case-insensitive match"},
"-A":{"type":"integer","description":"Lines of context after each match (content mode only)"},
"-B":{"type":"integer","description":"Lines of context before each match (content mode only)"},
"-C":{"type":"integer","description":"Lines of context before and after each match; overrides -A/-B (content mode only)"}
},
"required":["pattern"],
"additionalProperties":false
}`),
	}
}

func (grepHandler) Invoke(_ context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Pattern    string `json:"pattern"`
		Glob       string `json:"glob"`
		OutputMode string `json:"output_mode"`
		IgnoreCase bool   `json:"-i"`
		After      int    `json:"-A"`
		Before     int    `json:"-B"`
		Context    int    `json:"-C"`
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
	mode := args.OutputMode
	if mode == "" {
		mode = "files_with_matches"
	}
	if mode != "content" && mode != "files_with_matches" && mode != "count" {
		return `output_mode must be "content", "files_with_matches", or "count"`, true, nil
	}
	before, after := args.Before, args.After
	if args.Context > 0 {
		before, after = args.Context, args.Context
	}

	pat := args.Pattern
	if args.IgnoreCase {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return "invalid pattern: " + err.Error(), true, nil
	}

	var globRe *regexp.Regexp
	// A slashless glob (e.g. "*.go") matches the basename at any depth
	// (ripgrep/native Grep rule); a glob containing "/" matches the full
	// relative path.
	globBasename := args.Glob != "" && !strings.Contains(filepath.ToSlash(args.Glob), "/")
	if args.Glob != "" {
		globRe, err = globToRegexp(args.Glob)
		if err != nil {
			return "invalid glob: " + err.Error(), true, nil
		}
	}

	root, err := filepath.EvalSymlinks(env.WorkDir)
	if err != nil {
		return err.Error(), true, nil
	}

	type fileResult struct {
		rel     string
		matches []string
		count   int
		mtime   int64
	}
	var results []fileResult

	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if globRe != nil {
			target := rel
			if globBasename {
				target = filepath.Base(rel)
			}
			if !globRe.MatchString(target) {
				return nil
			}
		}
		info, infoErr := d.Info()
		if infoErr != nil || info.Size() > grepMaxFileBytes {
			return nil
		}
		lines, readErr := readGrepLines(p)
		if readErr != nil {
			return nil // skip unreadable/binary files
		}
		var matchedLines []int
		for i, line := range lines {
			if re.MatchString(line) {
				matchedLines = append(matchedLines, i)
			}
		}
		if len(matchedLines) == 0 {
			return nil
		}
		fr := fileResult{rel: rel, count: len(matchedLines), mtime: info.ModTime().UnixNano()}
		if mode == "content" {
			fr.matches = formatGrepContent(lines, matchedLines, before, after)
		}
		results = append(results, fr)
		return nil
	})
	if walkErr != nil {
		return walkErr.Error(), true, nil
	}

	if len(results) == 0 {
		return "no matches for " + args.Pattern, false, nil
	}
	sort.Slice(results, func(i, j int) bool { return results[i].mtime > results[j].mtime })

	var out strings.Builder
	switch mode {
	case "files_with_matches":
		for _, r := range results {
			out.WriteString(r.rel)
			out.WriteString("\n")
		}
	case "count":
		for _, r := range results {
			fmt.Fprintf(&out, "%s:%d\n", r.rel, r.count)
		}
	case "content":
		for _, r := range results {
			out.WriteString(r.rel)
			out.WriteString(":\n")
			for _, l := range r.matches {
				out.WriteString(l)
				out.WriteString("\n")
			}
		}
	}
	text := out.String()
	if len(text) > grepOutputCap {
		text = text[:grepOutputCap] + "\n… output truncated"
	}
	return strings.TrimRight(text, "\n"), false, nil
}

// readGrepLines reads a file's full content as lines; errors (including a
// NUL-byte binary heuristic) cause the caller to skip the file.
func readGrepLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.ContainsRune(string(data), 0) {
		return nil, fmt.Errorf("binary file")
	}
	return strings.Split(string(data), "\n"), nil
}

// formatGrepContent renders matched lines (cat -n shaped: "N\tline") with
// before/after context, separating non-adjacent windows with "--" like
// native grep -A/-B/-C.
func formatGrepContent(lines []string, matched []int, before, after int) []string {
	include := make(map[int]bool)
	for _, m := range matched {
		for i := m - before; i <= m+after; i++ {
			if i >= 0 && i < len(lines) {
				include[i] = true
			}
		}
	}
	idx := make([]int, 0, len(include))
	for i := range include {
		idx = append(idx, i)
	}
	sort.Ints(idx)

	out := make([]string, 0, len(idx))
	prev := -2
	for _, i := range idx {
		if i != prev+1 && prev != -2 {
			out = append(out, "--")
		}
		out = append(out, fmt.Sprintf("%6d\t%s", i+1, lines[i]))
		prev = i
	}
	return out
}
