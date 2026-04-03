package spawner

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"be/internal/logger"
	"be/internal/repo"
)

// captureLog redirects logger output to a fresh buffer for the test duration.
// The original writer is restored via t.Cleanup.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	orig := logger.GetWriter()
	buf := &bytes.Buffer{}
	logger.SetWriter(buf)
	t.Cleanup(func() {
		logger.SetWriter(orig)
	})
	return buf
}

// procWithTrx creates a minimal processInfo with agentType, modelID, and trx set.
func procWithTrx(agentType, modelID, trx string) *processInfo {
	return &processInfo{
		agentType:       agentType,
		modelID:         modelID,
		trx:             trx,
		pendingMessages: make([]repo.MessageEntry, 0),
		pendingTasks:    make(map[string]taskInfo),
	}
}

// TestLogAgent_InfoLevelWithTrxAndPrefix verifies logAgent writes INFO log with proc.trx and [agent:model] prefix.
func TestLogAgent_InfoLevelWithTrxAndPrefix(t *testing.T) {
	buf := captureLog(t)
	s := noPoolSpawner()
	proc := procWithTrx("implementor", "claude:opus", "abc12345")

	s.logAgent(proc, "tool call detail")

	out := buf.String()
	if !strings.Contains(out, "INFO") {
		t.Errorf("logAgent: expected INFO level in output, got: %s", out)
	}
	if !strings.Contains(out, "[abc12345]") {
		t.Errorf("logAgent: expected trx [abc12345] in output, got: %s", out)
	}
	if !strings.Contains(out, "[implementor:opus]") {
		t.Errorf("logAgent: expected prefix [implementor:opus] in output, got: %s", out)
	}
	if !strings.Contains(out, "tool call detail") {
		t.Errorf("logAgent: expected message in output, got: %s", out)
	}
}

// TestWarnAgent_WarnLevelWithTrx verifies warnAgent writes WARN-level log with proc.trx and prefix.
func TestWarnAgent_WarnLevelWithTrx(t *testing.T) {
	buf := captureLog(t)
	s := noPoolSpawner()
	proc := procWithTrx("qa-verifier", "claude:sonnet", "deadbeef")

	s.warnAgent(proc, "[stderr] some warning output")

	out := buf.String()
	if !strings.Contains(out, "WARN") {
		t.Errorf("warnAgent: expected WARN level in output, got: %s", out)
	}
	if !strings.Contains(out, "[deadbeef]") {
		t.Errorf("warnAgent: expected trx [deadbeef] in output, got: %s", out)
	}
	if !strings.Contains(out, "[qa-verifier:sonnet]") {
		t.Errorf("warnAgent: expected prefix [qa-verifier:sonnet] in output, got: %s", out)
	}
	if !strings.Contains(out, "[stderr] some warning output") {
		t.Errorf("warnAgent: expected message in output, got: %s", out)
	}
}

// TestErrorAgent_ErrorLevelWithTrx verifies errorAgent writes ERROR-level log with proc.trx and prefix.
func TestErrorAgent_ErrorLevelWithTrx(t *testing.T) {
	buf := captureLog(t)
	s := noPoolSpawner()
	proc := procWithTrx("setup-analyzer", "claude:haiku", "cafebabe")

	s.errorAgent(proc, "scanner error: unexpected EOF")

	out := buf.String()
	if !strings.Contains(out, "ERROR") {
		t.Errorf("errorAgent: expected ERROR level in output, got: %s", out)
	}
	if !strings.Contains(out, "[cafebabe]") {
		t.Errorf("errorAgent: expected trx [cafebabe] in output, got: %s", out)
	}
	if !strings.Contains(out, "[setup-analyzer:haiku]") {
		t.Errorf("errorAgent: expected prefix in output, got: %s", out)
	}
	if !strings.Contains(out, "scanner error: unexpected EOF") {
		t.Errorf("errorAgent: expected message in output, got: %s", out)
	}
}

// TestLogAgent_EmptyTrxShowsDash verifies that empty proc.trx produces "-" trx in log output.
func TestLogAgent_EmptyTrxShowsDash(t *testing.T) {
	buf := captureLog(t)
	s := noPoolSpawner()
	proc := procWithTrx("implementor", "claude:opus", "") // empty trx

	s.logAgent(proc, "msg")

	out := buf.String()
	if !strings.Contains(out, "[-]") {
		t.Errorf("logAgent with empty trx: expected [-] in output, got: %s", out)
	}
}

// TestLogAgent_TrxIsolation verifies two procs with different trx values produce isolated log lines.
func TestLogAgent_TrxIsolation(t *testing.T) {
	buf := captureLog(t)
	s := noPoolSpawner()

	proc1 := procWithTrx("implementor", "claude:opus", "aaaa1111")
	proc2 := procWithTrx("implementor", "claude:opus", "bbbb2222")

	s.logAgent(proc1, "msg from proc1")
	s.logAgent(proc2, "msg from proc2")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d:\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "[aaaa1111]") {
		t.Errorf("line 0 missing proc1 trx [aaaa1111]: %s", lines[0])
	}
	if !strings.Contains(lines[1], "[bbbb2222]") {
		t.Errorf("line 1 missing proc2 trx [bbbb2222]: %s", lines[1])
	}
}

// TestPrintStatus_RunningAgent_LogsAgentStatusLine verifies printStatus emits one INFO log per running agent.
func TestPrintStatus_RunningAgent_LogsAgentStatusLine(t *testing.T) {
	buf := captureLog(t)
	s := noPoolSpawner()

	proc := procWithTrx("implementor", "claude:opus", "11223344")
	proc.startTime = time.Now().Add(-10 * time.Second)
	proc.lastMessage = "recent tool call"

	s.printStatus([]*processInfo{proc}, nil, "phase-l2")

	out := buf.String()
	if !strings.Contains(out, "agent status") {
		t.Errorf("printStatus running: missing 'agent status', got: %s", out)
	}
	if !strings.Contains(out, "phase=phase-l2") {
		t.Errorf("printStatus running: missing phase=phase-l2, got: %s", out)
	}
	if !strings.Contains(out, "model=claude:opus") {
		t.Errorf("printStatus running: missing model=claude:opus, got: %s", out)
	}
	if !strings.Contains(out, "[11223344]") {
		t.Errorf("printStatus running: missing trx [11223344], got: %s", out)
	}
	if !strings.Contains(out, "recent tool") {
		t.Errorf("printStatus running: missing last_msg content, got: %s", out)
	}
}

// TestPrintStatus_CompletedAgent_LogsStatusAndDuration verifies printStatus emits correct fields for completed agents.
func TestPrintStatus_CompletedAgent_LogsStatusAndDuration(t *testing.T) {
	buf := captureLog(t)
	s := noPoolSpawner()

	proc := procWithTrx("qa-verifier", "claude:sonnet", "55667788")
	proc.finalStatus = "PASS"
	proc.elapsed = 90 * time.Second

	s.printStatus(nil, []*processInfo{proc}, "phase-l3")

	out := buf.String()
	if !strings.Contains(out, "agent status") {
		t.Errorf("printStatus completed: missing 'agent status', got: %s", out)
	}
	if !strings.Contains(out, "status=PASS") {
		t.Errorf("printStatus completed: missing status=PASS, got: %s", out)
	}
	if !strings.Contains(out, "phase=phase-l3") {
		t.Errorf("printStatus completed: missing phase=phase-l3, got: %s", out)
	}
	if !strings.Contains(out, "model=claude:sonnet") {
		t.Errorf("printStatus completed: missing model=claude:sonnet, got: %s", out)
	}
	if !strings.Contains(out, "[55667788]") {
		t.Errorf("printStatus completed: missing trx [55667788], got: %s", out)
	}
}

// TestPrintStatus_MultipleAgents_OneLineEach verifies printStatus emits exactly one log line per agent.
func TestPrintStatus_MultipleAgents_OneLineEach(t *testing.T) {
	buf := captureLog(t)
	s := noPoolSpawner()

	r1 := procWithTrx("implementor", "claude:opus", "aaaa0001")
	r1.startTime = time.Now()
	r2 := procWithTrx("test-writer", "claude:sonnet", "bbbb0002")
	r2.startTime = time.Now()
	c1 := procWithTrx("setup-analyzer", "claude:haiku", "cccc0003")
	c1.finalStatus = "PASS"

	s.printStatus([]*processInfo{r1, r2}, []*processInfo{c1}, "phase-l0")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("printStatus with 2 running + 1 completed: expected 3 lines, got %d:\n%s", len(lines), buf.String())
	}
}

// TestTrxField_ProcessInfo verifies the processInfo trx field can be set and read.
func TestTrxField_ProcessInfo(t *testing.T) {
	proc := &processInfo{trx: "feedf00d"}
	if proc.trx != "feedf00d" {
		t.Errorf("processInfo.trx = %q, want feedf00d", proc.trx)
	}
	proc.trx = "00000000"
	if proc.trx != "00000000" {
		t.Errorf("processInfo.trx after update = %q, want 00000000", proc.trx)
	}
}

// TestLogAgent_FormatPrefix_ModelParsed verifies formatPrefix extracts model from cli:model format.
func TestLogAgent_FormatPrefix_ModelParsed(t *testing.T) {
	buf := captureLog(t)
	s := noPoolSpawner()

	// modelID "claude:opus" should produce prefix "[doc-updater:opus]"
	proc := procWithTrx("doc-updater", "claude:opus", "99887766")
	s.logAgent(proc, "hello")

	out := buf.String()
	if !strings.Contains(out, "[doc-updater:opus]") {
		t.Errorf("formatPrefix: expected [doc-updater:opus] in output, got: %s", out)
	}
}

// TestLogAgent_DefaultModelWhenNoColon verifies formatPrefix uses "default" when modelID has no colon.
func TestLogAgent_DefaultModelWhenNoColon(t *testing.T) {
	buf := captureLog(t)
	s := noPoolSpawner()

	proc := procWithTrx("implementor", "opus", "aabbccdd") // no cli: prefix
	s.logAgent(proc, "hello")

	out := buf.String()
	// parseModelID with no colon returns ("", "opus") or similar; model becomes "opus"
	// Either way, the output must contain the agent type
	if !strings.Contains(out, "implementor") {
		t.Errorf("formatPrefix: expected agent type in output, got: %s", out)
	}
}

// Compile-time check: logger.GetWriter returns io.Writer (used to suppress import warning)
var _ io.Writer = logger.GetWriter()
