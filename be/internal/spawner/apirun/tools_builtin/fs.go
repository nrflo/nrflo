package tools_builtin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"be/internal/spawner/apirun"
)

// FSTools returns the native filesystem/shell handlers offered toward Claude
// Code parity: read_file / edit_file / write_file / glob / grep (workdir-
// jailed) plus bash / bash_output / kill_shell (foreground and background
// shell). Deliberately NOT part of Builtins(): they execute model-authored
// operations on the server's filesystem, so call sites merge them only when
// the `api_native_tools_enabled` global setting is on — and the console api
// engine additionally wraps edit_file/write_file/bash in a human approval
// gate (spawner/console_engine_api_approval.go). bash additionally runs
// through ToolEnv.SafetyCheck (a script gate, resolved by the spawner) when
// wired.
func FSTools() map[string]apirun.ToolHandler {
	return map[string]apirun.ToolHandler{
		"read_file":   readFileHandler{},
		"edit_file":   editFileHandler{},
		"write_file":  writeFileHandler{},
		"glob":        globHandler{},
		"grep":        grepHandler{},
		"bash":        bashHandler{},
		"bash_output": bashOutputHandler{},
		"kill_shell":  killShellHandler{},
	}
}

// FSApprovalRequired reports whether a native fs tool must go through the
// console human approval gate. Gated: edit_file, write_file, bash — each
// mutates the workdir or spawns a process. Exempt: read_file/glob/grep are
// read-only within the workdir jail; bash_output/kill_shell only observe or
// terminate a background shell that an already-approved bash created, so
// gating them would strand an approved shell behind a second, redundant
// prompt rather than protect anything new.
func FSApprovalRequired(name string) bool {
	return name == "edit_file" || name == "write_file" || name == "bash"
}

// resolveFSPath jails path inside env.WorkDir: relative paths resolve
// against it, absolute paths must already be inside it, and symlinks are
// resolved on the workdir AND every existing ancestor of the target before
// the prefix check — a symlink escape inside the tree must not pass.
func resolveFSPath(env apirun.ToolEnv, path string) (string, error) {
	if env.WorkDir == "" {
		return "", fmt.Errorf("no working directory configured for this session")
	}
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	root, err := filepath.EvalSymlinks(env.WorkDir)
	if err != nil {
		return "", fmt.Errorf("resolve workdir: %w", err)
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	abs = filepath.Clean(abs)

	// Resolve the deepest existing ancestor so symlinks inside the tree
	// cannot point out of it; the not-yet-existing tail (file being created)
	// is re-appended verbatim.
	resolved, tail := abs, ""
	for {
		r, err := filepath.EvalSymlinks(resolved)
		if err == nil {
			resolved = filepath.Join(r, tail)
			break
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve path: %w", err)
		}
		tail = filepath.Join(filepath.Base(resolved), tail)
		parent := filepath.Dir(resolved)
		if parent == resolved {
			return "", fmt.Errorf("resolve path: no existing ancestor")
		}
		resolved = parent
	}

	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the working directory", path)
	}
	return resolved, nil
}
