package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotenv(t *testing.T) {
	t.Parallel()
	in := strings.Join([]string{
		"# a comment",
		"",
		"   ",
		"EXA_API_KEY=exa_plain",
		`JINA_API_KEY="jina_dquoted"`,
		"SINGLE='sq'",
		"export EXPORTED=val",
		"  SPACED  =  trimmed  ",
		"TOKEN=ab=cd",     // only first '=' splits
		"# commented=out", // comment, not a pair
		"NOEQUALS",        // malformed, skipped
		"=leadingeq",      // empty key, skipped
		"BAD-KEY=x",       // invalid env name, skipped
		"1LEADINGDIGIT=x", // invalid env name, skipped
	}, "\n")

	got := parseDotenv(strings.NewReader(in))
	want := []envPair{
		{"EXA_API_KEY", "exa_plain"},
		{"JINA_API_KEY", "jina_dquoted"},
		{"SINGLE", "sq"},
		{"EXPORTED", "val"},
		{"SPACED", "trimmed"},
		{"TOKEN", "ab=cd"},
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d pairs, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("pair[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestLoadDotenv_MissingFile(t *testing.T) {
	keys, err := loadDotenv(filepath.Join(t.TempDir(), "nope.env"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if keys != nil {
		t.Errorf("missing file applied %v, want nil", keys)
	}
}

func TestLoadDotenv_AppliesAndRespectsExistingEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "NRFLO_DOTENV_FRESH=fromfile\nNRFLO_DOTENV_PRESET=fromfile\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// PRESET is already in the real env → the file must NOT override it.
	t.Setenv("NRFLO_DOTENV_PRESET", "fromenv")
	// FRESH is set by loadDotenv → clean it up so the test leaves no residue.
	t.Cleanup(func() { os.Unsetenv("NRFLO_DOTENV_FRESH") })

	applied, err := loadDotenv(path)
	if err != nil {
		t.Fatalf("loadDotenv: %v", err)
	}
	if got := os.Getenv("NRFLO_DOTENV_FRESH"); got != "fromfile" {
		t.Errorf("FRESH = %q, want fromfile", got)
	}
	if got := os.Getenv("NRFLO_DOTENV_PRESET"); got != "fromenv" {
		t.Errorf("PRESET = %q, want fromenv (real env wins)", got)
	}
	if len(applied) != 1 || applied[0] != "NRFLO_DOTENV_FRESH" {
		t.Errorf("applied = %v, want [NRFLO_DOTENV_FRESH] (preset skipped)", applied)
	}
}
