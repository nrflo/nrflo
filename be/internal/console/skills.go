package console

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/types"
)

// skillMeta is one discovered project skill.
type skillMeta struct {
	Name        string
	Description string
	Path        string // absolute path to the skill's SKILL.md
}

// discoverSkills scans <rootDir>/.claude/skills/*/SKILL.md, parsing each
// file's frontmatter for name/description (dir-name fallback when name is
// absent). Read-only, per-request, no watcher — a directory without a
// SKILL.md is silently ignored, never an error. Missing/unreadable rootDir
// or skills dir returns an empty slice, never an error.
func discoverSkills(rootDir string) []skillMeta {
	if rootDir == "" {
		return nil
	}
	skillsDir := filepath.Join(rootDir, ".claude", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	var out []skillMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		raw, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}
		name, desc := parseSkillFrontmatter(raw)
		if name == "" {
			name = entry.Name()
		}
		out = append(out, skillMeta{Name: name, Description: desc, Path: skillPath})
	}
	return out
}

// parseSkillFrontmatter hand-rolls a minimal YAML-frontmatter reader for
// `---\n...\n---` blocks: only the top-level `name:`/`description:` scalars
// are recognized. Handles a plain scalar, a quoted scalar, and a folded (`>`
// or `|`) block scalar for description (subsequent indented lines joined
// with spaces) — good enough for dispatch, which only needs name+body;
// description is display-only.
func parseSkillFrontmatter(raw []byte) (name, description string) {
	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}
	i := 1
	inFold := false
	foldIndent := -1
	for ; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "---" {
			break
		}
		if inFold {
			trimmed := strings.TrimRight(line, " \t")
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			if strings.TrimSpace(line) == "" || (foldIndent >= 0 && indent < foldIndent) {
				inFold = false
			} else {
				if foldIndent < 0 {
					foldIndent = indent
				}
				if description != "" {
					description += " "
				}
				description += strings.TrimSpace(trimmed)
				continue
			}
		}
		key, value, ok := splitFrontmatterLine(line)
		if !ok {
			continue
		}
		switch key {
		case "name":
			name = unquoteScalar(value)
		case "description":
			if value == ">" || value == "|" || value == ">-" || value == "|-" {
				inFold = true
				foldIndent = -1
				description = ""
				continue
			}
			description = unquoteScalar(value)
		}
	}
	return name, description
}

// splitFrontmatterLine splits "key: value" at the top level (no leading
// indent); ok is false for indented/blank/malformed lines.
func splitFrontmatterLine(line string) (key, value string, ok bool) {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		return "", "", false
	}
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

// unquoteScalar strips a single layer of matching quotes from a YAML scalar.
func unquoteScalar(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

// readSkillBody returns the SKILL.md content after the closing frontmatter
// `---` delimiter, trimmed. Returns "" when the file is unreadable or has no
// frontmatter block.
func readSkillBody(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := string(raw)
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return strings.TrimSpace(content)
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
		}
	}
	return ""
}

// matchSkill parses a leading "/name args" out of text and looks it up
// against skills by name. Returns nil for non-slash text, or a "/name" with
// no matching skill (unmatched/unknown "/text" is sent verbatim by the
// caller — never an error).
func matchSkill(skills []skillMeta, text string) *spawner.SkillMatch {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}
	rest := trimmed[1:]
	name := rest
	args := ""
	if idx := strings.IndexAny(rest, " \t\n"); idx >= 0 {
		name = rest[:idx]
		args = strings.TrimSpace(rest[idx:])
	}
	if name == "" {
		return nil
	}
	for _, sk := range skills {
		if sk.Name == name {
			return &spawner.SkillMatch{Name: sk.Name, Path: sk.Path, Body: readSkillBody(sk.Path), Args: args}
		}
	}
	return nil
}

// resolveSkill discovers rootDir's project skills and matches text's leading
// "/name" against them — the seam SendMessage uses to dispatch a turn to the
// engine (chat_service_turn.go).
func (s *ChatService) resolveSkill(rootDir, text string) *spawner.SkillMatch {
	return matchSkill(discoverSkills(rootDir), text)
}

// ListSkills resolves projectID's root_path and returns its discovered
// project skills for the "/" suggestion dropdown (GET
// /api/v1/console/skills). Mirrors Catalog's existence check
// (chat_catalog.go) so an unknown project maps to
// service.ErrConsoleProjectNotFound; an empty/invalid root_path on a known
// project yields an empty list, never an error.
func (s *ChatService) ListSkills(projectID string) ([]types.ConsoleSkill, error) {
	exists, err := repo.NewProjectRepo(s.deps.Pool, s.deps.Clock).Exists(projectID)
	if err != nil {
		return nil, fmt.Errorf("check console project: %w", err)
	}
	if !exists {
		return nil, service.ErrConsoleProjectNotFound
	}
	project, err := repo.NewProjectRepo(s.deps.Pool, s.deps.Clock).Get(projectID)
	if err != nil {
		return nil, fmt.Errorf("get console project: %w", err)
	}
	rootDir := ""
	if project.RootPath.Valid {
		rootDir = project.RootPath.String
	}
	metas := discoverSkills(rootDir)
	out := make([]types.ConsoleSkill, 0, len(metas))
	for _, m := range metas {
		out = append(out, types.ConsoleSkill{Name: m.Name, Description: m.Description})
	}
	return out, nil
}
