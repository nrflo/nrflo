package tools_builtin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"be/internal/spawner/apirun"
)

// FSTools returns the native filesystem/shell handlers offered toward Claude
// Code parity: read_file / glob / grep resolve workdir-relative but are NOT
// jailed to it (matching Claude Code's own Read/Glob/Grep, which have no cwd
// restriction) — the read jail was never a security boundary anyway once
// `bash` is grantable to the same agent. edit_file / write_file stay jailed
// to env.WorkDir via resolveFSPath, plus bash / bash_output / kill_shell
// (foreground and background shell). Deliberately NOT part of Builtins():
// they execute model-authored operations on the server's filesystem, so call
// sites merge them only when the `api_native_tools_enabled` global setting is
// on — and the console api engine additionally wraps edit_file/write_file/bash
// in a human approval gate (spawner/console_engine_api_approval.go). bash
// additionally runs through ToolEnv.SafetyCheck (a script gate, resolved by
// the spawner) when wired.
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
// mutates the workdir or spawns a process. Exempt: read_file/glob/grep only
// read (unjailed, workdir-relative); bash_output/kill_shell only observe or
// terminate a background shell that an already-approved bash created, so
// gating them would strand an approved shell behind a second, redundant
// prompt rather than protect anything new.
func FSApprovalRequired(name string) bool {
	return name == "edit_file" || name == "write_file" || name == "bash"
}

// resolveReadPathRoot resolves path against env.WorkDir the same way
// resolveFSPath does — relative-join, Clean, then symlink-resolve the
// deepest existing ancestor — but stops short of the workdir-prefix check,
// returning both the resolved path and the resolved workdir root so callers
// can apply their own policy (resolveFSPath's jail check, or glob/grep's
// relative-vs-absolute display choice).
func resolveReadPathRoot(env apirun.ToolEnv, path string) (abs, root string, err error) {
	if path == "" {
		return "", "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) && env.WorkDir == "" {
		return "", "", fmt.Errorf("no working directory configured for this session")
	}
	if env.WorkDir != "" {
		root, err = filepath.EvalSymlinks(env.WorkDir)
		if err != nil {
			return "", "", fmt.Errorf("resolve workdir: %w", err)
		}
	}
	abs = path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	abs = filepath.Clean(abs)

	// Resolve the deepest existing ancestor so symlinks inside the tree
	// resolve to their real target; the not-yet-existing tail (file being
	// created) is re-appended verbatim.
	resolved, tail := abs, ""
	for {
		r, evalErr := filepath.EvalSymlinks(resolved)
		if evalErr == nil {
			resolved = filepath.Join(r, tail)
			break
		}
		if !os.IsNotExist(evalErr) {
			return "", "", fmt.Errorf("resolve path: %w", evalErr)
		}
		tail = filepath.Join(filepath.Base(resolved), tail)
		parent := filepath.Dir(resolved)
		if parent == resolved {
			return "", "", fmt.Errorf("resolve path: no existing ancestor")
		}
		resolved = parent
	}
	return resolved, root, nil
}

// resolveReadPath resolves path for a read-only tool (read_file/glob/grep):
// workdir-relative, but an absolute path is honored anywhere on disk — reads
// are Claude Code parity, not a jail. A literal "~/x" is not special-cased:
// it joins under the workdir like any other relative path and fails as
// ENOENT, which is out of scope here.
func resolveReadPath(env apirun.ToolEnv, path string) (string, error) {
	abs, _, err := resolveReadPathRoot(env, path)
	return abs, err
}

// resolveFSPath jails path inside env.WorkDir: relative paths resolve
// against it, absolute paths must already be inside it, and symlinks are
// resolved on the workdir AND every existing ancestor of the target before
// the prefix check — a symlink escape inside the tree must not pass. Used
// by mutating tools (edit_file/write_file) and findings_add_from_file, which
// stays jailed regardless of whether bash is granted to the calling agent.
func resolveFSPath(env apirun.ToolEnv, path string) (string, error) {
	if env.WorkDir == "" {
		return "", fmt.Errorf("no working directory configured for this session")
	}
	resolved, root, err := resolveReadPathRoot(env, path)
	if err != nil {
		return "", err
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the working directory", path)
	}
	return resolved, nil
}

// insideWorkdir reports whether resolved root sits at or inside env.WorkDir
// (symlink-resolved), for glob/grep to choose relative-to-workdir vs.
// absolute output shape for a caller-supplied search root.
func insideWorkdir(env apirun.ToolEnv, root string) bool {
	workdirRoot, err := filepath.EvalSymlinks(env.WorkDir)
	if err != nil {
		return false
	}
	return root == workdirRoot || strings.HasPrefix(root, workdirRoot+string(filepath.Separator))
}
