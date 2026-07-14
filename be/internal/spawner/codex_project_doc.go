package spawner

import "fmt"

// codexProjectDocFallbackNames is the root-scope `project_doc_fallback_filenames`
// list codex uses to locate a project doc (first existing name wins per
// directory). It lets a project with no AGENTS.md have its root CLAUDE.md
// loaded as the codex project doc.
//
// Scope: codex walks only the ANCESTORS of its cwd, and every nrflo spawn
// sets cwd = Config.ProjectRoot, so in practice this loads exactly one file —
// the repo-root doc. Nested package CLAUDE.mds are NOT delivered by this;
// see REFERENCE.md § Codex app-server backend.
//
// Delivered as an argv `-c` override (codexProjectDocArgs), NOT appended to
// the per-session config.toml: a key appended after a table header is
// silently absorbed into that table, and a duplicate root key is a hard parse
// error ("duplicate key") if the user's own ~/.codex/config.toml already sets
// it. A `-c` override is accepted by app-server and wins over a conflicting
// config.toml value.
var codexProjectDocFallbackNames = []string{"AGENTS.md", "CLAUDE.md"}

// codexProjectDocArgs returns the `-c project_doc_fallback_filenames=[...]`
// pair for appServerArgs(), rendered from codexProjectDocFallbackNames so the
// list has one source of truth.
func codexProjectDocArgs() []string {
	value := "project_doc_fallback_filenames=["
	for i, name := range codexProjectDocFallbackNames {
		if i > 0 {
			value += ","
		}
		value += fmt.Sprintf("%q", name)
	}
	value += "]"
	return []string{"-c", value}
}
