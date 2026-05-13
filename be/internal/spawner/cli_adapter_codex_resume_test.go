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

func TestCodexAdapter_BuildResumeCommand(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	env := []string{"HOME=/home/test", "PATH=/usr/bin"}
	opts := ResumeOptions{
		SessionID:       "019c7aa2-8427-7850-bfc9-c5539d7937a0",
		WorkDir:         "/tmp/workdir",
		Env:             env,
		Model:           "codex_gpt54_normal",
		MappedModel:     "gpt-5.4",
		ReasoningEffort: "medium",
	}

	cmd := adapter.BuildResumeCommand(opts)
	if cmd == nil {
		t.Fatal("BuildResumeCommand returned nil")
	}

	args := strings.Join(cmd.Args, " ")

	// Verify subcommand structure
	if !strings.Contains(args, "exec resume") {
		t.Errorf("args missing 'exec resume': %s", args)
	}
	if !strings.Contains(args, opts.SessionID) {
		t.Errorf("args missing session ID %q: %s", opts.SessionID, args)
	}
	if !strings.Contains(args, "--json") {
		t.Errorf("args missing --json: %s", args)
	}
	if !strings.Contains(args, "--model gpt-5.4") {
		t.Errorf("args missing --model gpt-5.4: %s", args)
	}
	if !strings.Contains(args, `model_reasoning_effort="medium"`) {
		t.Errorf("args missing model_reasoning_effort=medium: %s", args)
	}
	if !strings.Contains(args, "check_for_update_on_startup=false") {
		t.Errorf("args missing check_for_update_on_startup=false: %s", args)
	}
	if !strings.Contains(args, "--dangerously-bypass-approvals-and-sandbox") {
		t.Errorf("args missing --dangerously-bypass-approvals-and-sandbox: %s", args)
	}

	// Dir and Env must be set
	if cmd.Dir != "/tmp/workdir" {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, "/tmp/workdir")
	}
	if len(cmd.Env) != len(env) {
		t.Errorf("cmd.Env len = %d, want %d", len(cmd.Env), len(env))
	}

	// No positional prompt arg — prompt is delivered via stdin
	// The last arg should be the session ID (or a flag), not a free-form string
	lastArg := cmd.Args[len(cmd.Args)-1]
	if lastArg == opts.SessionID {
		// session ID is the positional for 'resume', that's expected
	} else if strings.HasPrefix(lastArg, "--") || strings.HasPrefix(lastArg, "-c") {
		// flag, fine
	} else if lastArg == "false" || lastArg == "true" {
		// value for a preceding flag
	} else {
		// The last arg shouldn't be a large user prompt blob
		if len(lastArg) > 200 {
			t.Errorf("BuildResumeCommand appears to include a positional prompt arg (len=%d): %q", len(lastArg), lastArg[:80])
		}
	}
}

func TestCodexAdapter_BuildResumeCommand_MapsModel(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	t.Run("MappedModel set, used verbatim", func(t *testing.T) {
		t.Parallel()
		cmd := adapter.BuildResumeCommand(ResumeOptions{
			SessionID:   "sess-1",
			Model:       "codex_gpt54_normal",
			MappedModel: "gpt-5.4-override",
		})
		args := strings.Join(cmd.Args, " ")
		if !strings.Contains(args, "--model gpt-5.4-override") {
			t.Errorf("expected MappedModel verbatim in args: %s", args)
		}
		if strings.Contains(args, "--model gpt-5.4 ") || strings.Contains(args, "--model gpt-5.3-codex") {
			t.Errorf("MappedModel should override MapModel, but got: %s", args)
		}
	})

	t.Run("MappedModel empty, falls back to MapModel", func(t *testing.T) {
		t.Parallel()
		cmd := adapter.BuildResumeCommand(ResumeOptions{
			SessionID:   "sess-2",
			Model:       "codex_gpt_normal",
			MappedModel: "",
		})
		args := strings.Join(cmd.Args, " ")
		// codex_gpt_normal maps to gpt-5.3-codex
		if !strings.Contains(args, "--model gpt-5.3-codex") {
			t.Errorf("expected MapModel fallback 'gpt-5.3-codex' in args: %s", args)
		}
	})
}
