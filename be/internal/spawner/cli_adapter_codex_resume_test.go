package spawner

import (
	"strings"
	"testing"
)

func TestCodexAdapter_SupportsResume(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}
	if !adapter.SupportsResume() {
		t.Error("SupportsResume() should be true")
	}
}

// TestCodexAdapter_BuildInteractiveCommand_WithResumeSessionID verifies that
// setting ResumeSessionID prepends "resume <id>" as the subcommand.
func TestCodexAdapter_BuildInteractiveCommand_WithResumeSessionID(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	opts := InteractiveSpawnOptions{
		Model:           "gpt-5.4",
		WorkDir:         "/tmp/workdir",
		ResumeSessionID: "019c7aa2-8427-7850-bfc9-c5539d7937a0",
		ReasoningEffort: "medium",
	}

	cmd := adapter.BuildInteractiveCommand(opts)
	if cmd == nil {
		t.Fatal("BuildInteractiveCommand returned nil")
	}

	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "resume") {
		t.Errorf("args missing 'resume' subcommand: %s", args)
	}
	if !strings.Contains(args, opts.ResumeSessionID) {
		t.Errorf("args missing resume session ID %q: %s", opts.ResumeSessionID, args)
	}
	if !strings.Contains(args, "--model gpt-5.4") {
		t.Errorf("args missing --model gpt-5.4: %s", args)
	}
	if !strings.Contains(args, "--dangerously-bypass-approvals-and-sandbox") {
		t.Errorf("args missing --dangerously-bypass-approvals-and-sandbox: %s", args)
	}

	if cmd.Dir != "/tmp/workdir" {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, "/tmp/workdir")
	}

	// "resume <id>" must be the leading subcommand (before --model etc.)
	resumeIdx := -1
	modelIdx := -1
	for i, a := range cmd.Args {
		if a == "resume" {
			resumeIdx = i
		}
		if a == "--model" {
			modelIdx = i
		}
	}
	if resumeIdx < 0 {
		t.Errorf("'resume' subcommand not found in args: %v", cmd.Args)
	}
	if modelIdx < 0 {
		t.Errorf("'--model' not found in args: %v", cmd.Args)
	}
	if resumeIdx > 0 && modelIdx > 0 && resumeIdx >= modelIdx {
		t.Errorf("'resume' subcommand (idx=%d) must appear before '--model' (idx=%d): %v", resumeIdx, modelIdx, cmd.Args)
	}
}

// TestCodexAdapter_BuildInteractiveCommand_NoResumeSessionID verifies that
// without ResumeSessionID the normal interactive form is used (no "resume" subcommand).
func TestCodexAdapter_BuildInteractiveCommand_NoResumeSessionID(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	opts := InteractiveSpawnOptions{
		Model:   "gpt-5.3-codex",
		WorkDir: "/tmp",
	}

	args := strings.Join(adapter.BuildInteractiveCommand(opts).Args, " ")

	// "resume" as a subcommand (first arg after binary) must not appear
	if strings.HasPrefix(args, "codex resume") {
		t.Errorf("BuildInteractiveCommand without ResumeSessionID must not start with 'codex resume': %s", args)
	}
}

// TestCodexAdapter_BuildInteractiveCommand_ResumeSessionID_MapsModel verifies
// the model field in the resume command is built from opts.Model as-is
// (the model is already mapped by the caller; adapter doesn't call MapModel again).
func TestCodexAdapter_BuildInteractiveCommand_ResumeSessionID_MapsModel(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	t.Run("pre-mapped model used verbatim", func(t *testing.T) {
		t.Parallel()
		opts := InteractiveSpawnOptions{
			Model:           "gpt-5.4",
			ResumeSessionID: "sess-1",
			WorkDir:         "/tmp",
		}
		args := strings.Join(adapter.BuildInteractiveCommand(opts).Args, " ")
		if !strings.Contains(args, "--model gpt-5.4") {
			t.Errorf("expected --model gpt-5.4 in args: %s", args)
		}
	})

	t.Run("alias model passed through unchanged", func(t *testing.T) {
		t.Parallel()
		opts := InteractiveSpawnOptions{
			Model:           "gpt-5.3-codex",
			ResumeSessionID: "sess-2",
			WorkDir:         "/tmp",
		}
		args := strings.Join(adapter.BuildInteractiveCommand(opts).Args, " ")
		if !strings.Contains(args, "--model gpt-5.3-codex") {
			t.Errorf("expected --model gpt-5.3-codex in args: %s", args)
		}
	})
}
