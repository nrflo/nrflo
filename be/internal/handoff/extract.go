package handoff

import (
	"encoding/json"
	"regexp"
	"strings"

	"be/internal/repo"
)

// Per-list caps on deterministic candidate extraction — generous enough to
// cover a real session's tool traffic without letting one chatty session
// dominate the Verified State byte budget.
const (
	maxRefs        = 40
	maxCommands    = 15
	maxTestResults = 10
	maxTicketIDs   = 20
)

// candidates holds deterministic, verbatim-copied extraction results from a
// session's message rows — never normalized, never rewritten. Resolution
// against the working tree happens later, in resolve.go.
type candidates struct {
	paths     []string
	tickets   []string
	commands  []string
	testLines []string
}

var (
	sourceExtRe = regexp.MustCompile(`\.(go|ts|tsx|js|jsx|py|rb|java|rs|c|cpp|h|hpp|sql|sh|yaml|yml|json|md|proto|graphql)$`)

	// ticketSlugRe matches nrflo's own ticket id shape (e.g. "nrworkflow-e346a7");
	// ticketUpperRe matches a JIRA-style shortcode (e.g. "PROJ-123").
	ticketSlugRe  = regexp.MustCompile(`\b[a-z][a-z0-9_]*-[0-9a-f]{6}\b`)
	ticketUpperRe = regexp.MustCompile(`\b[A-Z]{2,}-[0-9]+\b`)

	// testResultRes are anchored, verbatim-copied test-output signatures —
	// Go's `ok`/`FAIL`, Vitest/Jest's `PASS`/`Test Files:`, pytest's
	// "N passed/failed", and a generic non-zero shell exit.
	testResultRes = []*regexp.Regexp{
		regexp.MustCompile(`^ok\s`),
		regexp.MustCompile(`^(---\s+)?FAIL`),
		regexp.MustCompile(`^PASS`),
		regexp.MustCompile(`^(Tests|Test Files):`),
		regexp.MustCompile(`^\s*\d+ (passed|failed)`),
		regexp.MustCompile(`exit status \d+`),
	}
)

// extractFrom scans a session's message rows for candidate file paths,
// ticket IDs, commands, and test-result lines, trusting structured data
// over free text: (a) tool payload.input first, (b) "[Tool] detail" content
// second, (c) conservative regexes over free text last. Every match is
// copied verbatim — no normalization, no extension fixing, no case folding.
func extractFrom(msgs []repo.TailMessage) candidates {
	c := &candidateSink{}
	for _, m := range msgs {
		if m.Payload != "" && extractFromPayload(m.Payload, c) {
			continue
		}
		if extractFromToolContent(m.Content, c) {
			continue
		}
		extractFromFreeText(m.Content, c)
	}
	return c.candidates
}

// candidateSink dedupes and caps each extracted list as candidates are
// found, preserving first-seen order.
type candidateSink struct {
	candidates candidates
	seenPath   map[string]bool
	seenTicket map[string]bool
	seenCmd    map[string]bool
	seenTest   map[string]bool
}

func (s *candidateSink) addPath(p string) {
	p = strings.TrimSpace(p)
	if p == "" || len(s.candidates.paths) >= maxRefs {
		return
	}
	if s.seenPath == nil {
		s.seenPath = map[string]bool{}
	}
	if s.seenPath[p] {
		return
	}
	s.seenPath[p] = true
	s.candidates.paths = append(s.candidates.paths, p)
}

func (s *candidateSink) addTicket(t string) {
	if t == "" || len(s.candidates.tickets) >= maxTicketIDs {
		return
	}
	if s.seenTicket == nil {
		s.seenTicket = map[string]bool{}
	}
	if s.seenTicket[t] {
		return
	}
	s.seenTicket[t] = true
	s.candidates.tickets = append(s.candidates.tickets, t)
}

func (s *candidateSink) addCmd(cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" || len(s.candidates.commands) >= maxCommands {
		return
	}
	if s.seenCmd == nil {
		s.seenCmd = map[string]bool{}
	}
	if s.seenCmd[cmd] {
		return
	}
	s.seenCmd[cmd] = true
	s.candidates.commands = append(s.candidates.commands, cmd)
}

func (s *candidateSink) addTest(line string) {
	line = strings.TrimSpace(line)
	if line == "" || len(s.candidates.testLines) >= maxTestResults {
		return
	}
	if s.seenTest == nil {
		s.seenTest = map[string]bool{}
	}
	if s.seenTest[line] {
		return
	}
	s.seenTest[line] = true
	s.candidates.testLines = append(s.candidates.testLines, line)
}

// toolPayload mirrors the shape spawner.BuildToolInvokePayload
// (tool_format.go:196) writes to agent_messages.payload.
type toolPayload struct {
	Input map[string]interface{} `json:"input"`
}

// extractFromPayload reads structured tool input: file_path/filePath/path/
// notebook_path for a touched file, command for Bash. Returns whether a
// payload was successfully parsed, so the caller does not fall through to
// the weaker content-parse tier even when the payload carried no usable
// field.
func extractFromPayload(payload string, c *candidateSink) bool {
	var p toolPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil || p.Input == nil {
		return false
	}
	for _, key := range []string{"file_path", "filePath", "path", "notebook_path"} {
		if v, ok := p.Input[key].(string); ok && v != "" {
			c.addPath(v)
		}
	}
	if cmd, ok := p.Input["command"].(string); ok && cmd != "" {
		c.addCmd(cmd)
	}
	return true
}

// extractFromToolContent parses the "[ToolName] detail" shape written by
// spawner.FormatToolDetail (tool_format.go:60) for rows with no payload.
// Returns whether the row matched a known tool prefix.
func extractFromToolContent(content string, c *candidateSink) bool {
	if !strings.HasPrefix(content, "[") {
		return false
	}
	end := strings.Index(content, "]")
	if end < 0 {
		return false
	}
	tool := content[1:end]
	rest := strings.TrimPrefix(content[end+1:], " ")

	switch tool {
	case "Bash":
		c.addCmd(rest)
		return true
	case "Read", "Write", "Edit", "Glob", "Grep":
		if rest != "" {
			c.addPath(rest)
		}
		return true
	}
	return false
}

// extractFromFreeText applies conservative regexes to unstructured content
// (assistant text, user input, tool results/errors): a path candidate must
// carry a known source extension or an embedded slash, so a bare word never
// becomes a path. Test-result lines are matched anchored, verbatim.
func extractFromFreeText(content string, c *candidateSink) {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		for _, re := range testResultRes {
			if re.MatchString(trimmed) {
				c.addTest(trimmed)
				break
			}
		}
		for _, tok := range strings.Fields(trimmed) {
			tok = strings.Trim(tok, "`'\",;:()[]{}")
			if tok == "" {
				continue
			}
			if sourceExtRe.MatchString(tok) || strings.Contains(tok, "/") {
				c.addPath(tok)
			}
		}
		for _, m := range ticketSlugRe.FindAllString(trimmed, -1) {
			c.addTicket(m)
		}
		for _, m := range ticketUpperRe.FindAllString(trimmed, -1) {
			c.addTicket(m)
		}
	}
}
