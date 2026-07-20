package console

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSkill creates <root>/.claude/skills/<dir>/SKILL.md with body.
func writeSkill(t *testing.T, root, dir, body string) {
	t.Helper()
	skillDir := filepath.Join(root, ".claude", "skills", dir)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", skillDir, err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

// buildSkillTree lays out the fixture tree described by the plan: a folded
// (">-") description, a quoted description, a no-name dir-fallback skill,
// and a directory with no SKILL.md at all (ignored).
func buildSkillTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeSkill(t, root, "finalize", "---\nname: finalize\ndescription: >-\n  Close out a chunk\n  of work in one pass.\n---\nFINALIZE BODY\n")
	writeSkill(t, root, "do-release", "---\nname: do-release\ndescription: \"Cut a new release.\"\n---\nRELEASE BODY\n")
	writeSkill(t, root, "noname", "---\ndescription: no name here\n---\nNONAME BODY\n")
	if err := os.MkdirAll(filepath.Join(root, ".claude", "skills", "notaskill"), 0o755); err != nil {
		t.Fatalf("mkdir notaskill: %v", err)
	}
	return root
}

func TestDiscoverSkills(t *testing.T) {
	root := buildSkillTree(t)
	got := discoverSkills(root)

	byName := map[string]skillMeta{}
	for _, m := range got {
		byName[m.Name] = m
	}
	if len(got) != 3 {
		t.Fatalf("discoverSkills returned %d skills, want 3 (notaskill dir must be ignored): %+v", len(got), got)
	}

	finalize, ok := byName["finalize"]
	if !ok {
		t.Fatalf("missing finalize skill: %+v", got)
	}
	if want := "Close out a chunk of work in one pass."; finalize.Description != want {
		t.Errorf("finalize description = %q, want %q (folded '>-' scalar joined with spaces)", finalize.Description, want)
	}

	release, ok := byName["do-release"]
	if !ok {
		t.Fatalf("missing do-release skill: %+v", got)
	}
	if want := "Cut a new release."; release.Description != want {
		t.Errorf("do-release description = %q, want %q (quoted scalar unquoted)", release.Description, want)
	}

	noname, ok := byName["noname"]
	if !ok {
		t.Fatalf("missing dir-name-fallback skill: %+v", got)
	}
	if noname.Description != "no name here" {
		t.Errorf("noname description = %q, want %q", noname.Description, "no name here")
	}
}

func TestDiscoverSkills_MissingOrInvalidRoot_ReturnsEmptyNotError(t *testing.T) {
	if got := discoverSkills(""); got != nil {
		t.Errorf("discoverSkills(\"\") = %+v, want nil", got)
	}
	if got := discoverSkills(filepath.Join(t.TempDir(), "does-not-exist")); got != nil {
		t.Errorf("discoverSkills(missing root) = %+v, want nil", got)
	}
	// A root with no .claude/skills dir at all.
	if got := discoverSkills(t.TempDir()); got != nil {
		t.Errorf("discoverSkills(no skills dir) = %+v, want nil", got)
	}
}

func TestReadSkillBody_StripsFrontmatter(t *testing.T) {
	root := buildSkillTree(t)
	metas := discoverSkills(root)
	var finalizePath string
	for _, m := range metas {
		if m.Name == "finalize" {
			finalizePath = m.Path
		}
	}
	if finalizePath == "" {
		t.Fatal("finalize skill not found")
	}
	if got := readSkillBody(finalizePath); got != "FINALIZE BODY" {
		t.Errorf("readSkillBody(finalize) = %q, want %q", got, "FINALIZE BODY")
	}
}

func TestReadSkillBody_NoFrontmatter_ReturnsTrimmedContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "SKILL.md")
	if err := os.WriteFile(path, []byte("  just a body, no frontmatter  \n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := readSkillBody(path); got != "just a body, no frontmatter" {
		t.Errorf("readSkillBody = %q, want trimmed raw content", got)
	}
}

func TestReadSkillBody_UnreadableFile_ReturnsEmpty(t *testing.T) {
	if got := readSkillBody(filepath.Join(t.TempDir(), "missing.md")); got != "" {
		t.Errorf("readSkillBody(missing) = %q, want empty", got)
	}
}

func TestMatchSkill(t *testing.T) {
	skills := []skillMeta{
		{Name: "finalize", Path: writeStandaloneSkill(t, "FINALIZE BODY")},
	}

	t.Run("matched with args", func(t *testing.T) {
		m := matchSkill(skills, "/finalize extra args")
		if m == nil {
			t.Fatal("matchSkill returned nil, want a match")
		}
		if m.Name != "finalize" || m.Args != "extra args" || m.Body != "FINALIZE BODY" {
			t.Errorf("match = %+v, want Name=finalize Args=%q Body=%q", m, "extra args", "FINALIZE BODY")
		}
	})

	t.Run("matched no args", func(t *testing.T) {
		m := matchSkill(skills, "/finalize")
		if m == nil {
			t.Fatal("matchSkill returned nil, want a match")
		}
		if m.Args != "" {
			t.Errorf("Args = %q, want empty when the user typed no arguments", m.Args)
		}
	})

	t.Run("unknown skill", func(t *testing.T) {
		if m := matchSkill(skills, "/unknown x"); m != nil {
			t.Errorf("matchSkill(unknown) = %+v, want nil", m)
		}
	})

	t.Run("plain text", func(t *testing.T) {
		if m := matchSkill(skills, "plain text"); m != nil {
			t.Errorf("matchSkill(plain text) = %+v, want nil", m)
		}
	})

	t.Run("empty after slash", func(t *testing.T) {
		if m := matchSkill(skills, "/"); m != nil {
			t.Errorf("matchSkill(\"/\") = %+v, want nil", m)
		}
	})
}

// writeStandaloneSkill writes a bare SKILL.md (no frontmatter) with body and
// returns its path, for tests that only need matchSkill's Body lookup.
func writeStandaloneSkill(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}
