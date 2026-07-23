package handoff

import (
	"strconv"
	"testing"

	"be/internal/repo"
)

func TestExtractFrom_PayloadPath(t *testing.T) {
	msgs := []repo.TailMessage{
		{Category: "tool", Content: "[Read] /x/y.go", Payload: `{"input":{"file_path":"/x/y.go"}}`},
	}
	c := extractFrom(msgs)
	if len(c.paths) != 1 || c.paths[0] != "/x/y.go" {
		t.Errorf("paths = %v, want [/x/y.go]", c.paths)
	}
}

func TestExtractFrom_ContentFallback_PathCandidate(t *testing.T) {
	msgs := []repo.TailMessage{
		{Category: "tool", Content: "[Read] /x/y.go"},
	}
	c := extractFrom(msgs)
	if len(c.paths) != 1 || c.paths[0] != "/x/y.go" {
		t.Errorf("paths = %v, want [/x/y.go]", c.paths)
	}
}

func TestExtractFrom_BashCommand(t *testing.T) {
	msgs := []repo.TailMessage{
		{Category: "tool", Content: "[Bash] make test"},
	}
	c := extractFrom(msgs)
	if len(c.commands) != 1 || c.commands[0] != "make test" {
		t.Errorf("commands = %v, want [make test]", c.commands)
	}
}

func TestExtractFrom_BareWord_NotAPathCandidate(t *testing.T) {
	msgs := []repo.TailMessage{
		{Category: "assistant", Content: "looking at foo now"},
	}
	c := extractFrom(msgs)
	if len(c.paths) != 0 {
		t.Errorf("paths = %v, want empty for bare word with no slash/extension", c.paths)
	}
}

func TestExtractFrom_TestResultLines_VerbatimCapture(t *testing.T) {
	tests := []string{
		"--- FAIL: TestFoo",
		"ok  be/internal/handoff 0.4s",
		"Tests: 3 failed",
	}
	for _, line := range tests {
		msgs := []repo.TailMessage{{Category: "tool_result", Content: line}}
		c := extractFrom(msgs)
		if len(c.testLines) != 1 || c.testLines[0] != line {
			t.Errorf("for input %q: testLines = %v, want [%q]", line, c.testLines, line)
		}
	}
}

func TestExtractFrom_TicketIDShapes(t *testing.T) {
	msgs := []repo.TailMessage{
		{Category: "assistant", Content: "working on nrworkflow-e346a7 and PROJ-123 today"},
	}
	c := extractFrom(msgs)
	if !containsStr(c.tickets, "nrworkflow-e346a7") {
		t.Errorf("tickets = %v, want to contain nrworkflow-e346a7", c.tickets)
	}
	if !containsStr(c.tickets, "PROJ-123") {
		t.Errorf("tickets = %v, want to contain PROJ-123", c.tickets)
	}
}

func TestExtractFrom_DedupeAndOrderStability(t *testing.T) {
	msgs := []repo.TailMessage{
		{Category: "tool", Content: "[Bash] make test"},
		{Category: "tool", Content: "[Bash] make lint"},
		{Category: "tool", Content: "[Bash] make test"},
	}
	c := extractFrom(msgs)
	want := []string{"make test", "make lint"}
	if len(c.commands) != len(want) {
		t.Fatalf("commands = %v, want %v", c.commands, want)
	}
	for i, w := range want {
		if c.commands[i] != w {
			t.Errorf("commands[%d] = %q, want %q (order/dedupe)", i, c.commands[i], w)
		}
	}
}

func TestExtractFrom_PerListCaps(t *testing.T) {
	var msgs []repo.TailMessage
	for i := 0; i < maxCommands+10; i++ {
		msgs = append(msgs, repo.TailMessage{
			Category: "tool",
			Content:  "[Bash] cmd-" + itoa(i),
		})
	}
	c := extractFrom(msgs)
	if len(c.commands) != maxCommands {
		t.Errorf("commands len = %d, want cap %d", len(c.commands), maxCommands)
	}
}

func TestExtractFrom_RefsCap(t *testing.T) {
	var msgs []repo.TailMessage
	for i := 0; i < maxRefs+10; i++ {
		msgs = append(msgs, repo.TailMessage{
			Category: "tool",
			Payload:  `{"input":{"file_path":"/x/file-` + itoa(i) + `.go"}}`,
		})
	}
	c := extractFrom(msgs)
	if len(c.paths) != maxRefs {
		t.Errorf("paths len = %d, want cap %d", len(c.paths), maxRefs)
	}
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
