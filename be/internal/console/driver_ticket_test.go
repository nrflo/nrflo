package console

import (
	"strings"
	"testing"
)

func argvValueAfter(argv []string, flag string) (string, bool) {
	for i, a := range argv {
		if a == flag && i+1 < len(argv) {
			return argv[i+1], true
		}
	}
	return "", false
}

func TestTicketHint_EmptyWhenNoTicket(t *testing.T) {
	if h := ticketHint(""); h != "" {
		t.Errorf("ticketHint(\"\") = %q, want empty", h)
	}
	if h := ticketHint("NRF-42"); !strings.Contains(h, "NRF-42") || !strings.Contains(h, "ticket_id") {
		t.Errorf("ticketHint(NRF-42) = %q, want it to name the ticket and workflow_run's ticket_id", h)
	}
}

func TestClaudeDriver_Prepare_InjectsTicketHint(t *testing.T) {
	d := &claudeDriver{}
	spec, cleanup, err := d.Prepare(LaunchInput{NrfloPath: "/opt/nrflo_server", WorkDir: t.TempDir(), CurrentTicket: "NRF-42"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer cleanup()

	val, ok := argvValueAfter(spec.Argv, "--append-system-prompt")
	if !ok {
		t.Fatalf("Argv %v missing --append-system-prompt for a current ticket", spec.Argv)
	}
	if !strings.Contains(val, "NRF-42") {
		t.Errorf("--append-system-prompt value = %q, want it to name NRF-42", val)
	}
}

func TestClaudeDriver_Prepare_NoTicket_NoHintFlag(t *testing.T) {
	d := &claudeDriver{}
	spec, cleanup, err := d.Prepare(LaunchInput{NrfloPath: "/opt/nrflo_server", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer cleanup()
	if _, ok := argvValueAfter(spec.Argv, "--append-system-prompt"); ok {
		t.Errorf("Argv %v must not carry --append-system-prompt when there is no current ticket", spec.Argv)
	}
}

func TestCodexDriver_Prepare_DoesNotInjectHintIntoArgv(t *testing.T) {
	d := &codexDriver{}
	spec, cleanup, err := d.Prepare(LaunchInput{NrfloPath: "/opt/nrflo_server", WorkDir: t.TempDir(), CurrentTicket: "NRF-42"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer cleanup()
	// codex has no system-prompt-append channel; the ticket must not leak into
	// argv (the model gets it from ticket_current instead).
	for _, a := range spec.Argv {
		if strings.Contains(a, "NRF-42") {
			t.Errorf("codex Argv %v unexpectedly contains the ticket hint", spec.Argv)
		}
	}
}
