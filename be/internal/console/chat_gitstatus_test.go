package console

import (
	"errors"
	"testing"
)

var errNotAGitRepo = errors.New("not a git repository")

func TestParseNumstat(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		wantAdded   int
		wantDeleted int
		wantOK      bool
	}{
		{"empty output is clean", "", 0, 0, true},
		{"single file", "10\t3\tfoo.go", 10, 3, true},
		{"multiple files summed", "10\t3\tfoo.go\n5\t1\tbar.go", 15, 4, true},
		{"binary rows count as zero", "-\t-\timage.png\n2\t1\tfoo.go", 2, 1, true},
		{"malformed line omits", "not-a-numstat-line", 0, 0, false},
		{"non-numeric count omits", "abc\t1\tfoo.go", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, deleted, ok := parseNumstat(tt.output)
			if added != tt.wantAdded || deleted != tt.wantDeleted || ok != tt.wantOK {
				t.Errorf("parseNumstat(%q) = (%d, %d, %v), want (%d, %d, %v)",
					tt.output, added, deleted, ok, tt.wantAdded, tt.wantDeleted, tt.wantOK)
			}
		})
	}
}

func TestGitWorkdirStatus(t *testing.T) {
	orig := runGitCommand
	defer func() { runGitCommand = orig }()

	t.Run("clean repo on a branch", func(t *testing.T) {
		runGitCommand = func(dir string, args ...string) ([]byte, error) {
			if args[0] == "rev-parse" {
				return []byte("master\n"), nil
			}
			return []byte(""), nil
		}
		branch, added, deleted, ok := gitWorkdirStatus("/repo")
		if !ok || branch != "master" || added != 0 || deleted != 0 {
			t.Errorf("got (%q, %d, %d, %v), want (master, 0, 0, true)", branch, added, deleted, ok)
		}
	})

	t.Run("dirty repo on a branch", func(t *testing.T) {
		runGitCommand = func(dir string, args ...string) ([]byte, error) {
			if args[0] == "rev-parse" {
				return []byte("master\n"), nil
			}
			return []byte("20\t3\tfoo.go\n"), nil
		}
		branch, added, deleted, ok := gitWorkdirStatus("/repo")
		if !ok || branch != "master" || added != 20 || deleted != 3 {
			t.Errorf("got (%q, %d, %d, %v), want (master, 20, 3, true)", branch, added, deleted, ok)
		}
	})

	t.Run("detached HEAD uses short SHA", func(t *testing.T) {
		runGitCommand = func(dir string, args ...string) ([]byte, error) {
			if args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
				return []byte("HEAD\n"), nil
			}
			if args[0] == "rev-parse" && args[1] == "--short" {
				return []byte("a1b2c3d\n"), nil
			}
			return []byte(""), nil
		}
		branch, _, _, ok := gitWorkdirStatus("/repo")
		if !ok || branch != "a1b2c3d" {
			t.Errorf("got (%q, ok=%v), want (a1b2c3d, true)", branch, ok)
		}
	})

	t.Run("not a git repo omits segment", func(t *testing.T) {
		runGitCommand = func(dir string, args ...string) ([]byte, error) {
			return nil, errNotAGitRepo
		}
		_, _, _, ok := gitWorkdirStatus("/not-a-repo")
		if ok {
			t.Errorf("gitWorkdirStatus() ok = true, want false for a non-repo dir")
		}
	})

	t.Run("empty workdir omits segment", func(t *testing.T) {
		_, _, _, ok := gitWorkdirStatus("")
		if ok {
			t.Errorf("gitWorkdirStatus(\"\") ok = true, want false")
		}
	})

	t.Run("malformed numstat omits segment", func(t *testing.T) {
		runGitCommand = func(dir string, args ...string) ([]byte, error) {
			if args[0] == "rev-parse" {
				return []byte("master\n"), nil
			}
			return []byte("garbage"), nil
		}
		_, _, _, ok := gitWorkdirStatus("/repo")
		if ok {
			t.Errorf("gitWorkdirStatus() ok = true, want false for malformed numstat")
		}
	})
}
