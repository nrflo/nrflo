package spawner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

func TestAssistantBlocksText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"text blocks", `[{"type":"text","text":"API Error: 529 Overloaded."}]`, "API Error: 529 Overloaded."},
		{"two text blocks", `[{"type":"text","text":"a"},{"type":"text","text":"b"}]`, "a\nb"},
		{"tool_use only", `[{"type":"tool_use","name":"Bash"}]`, ""},
		{"plain string", `"hello"`, "hello"},
		{"empty", ``, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := assistantBlocksText([]byte(tc.raw)); got != tc.want {
				t.Errorf("assistantBlocksText(%s) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestClaudeTranscriptPath(t *testing.T) {
	t.Parallel()
	// CLAUDE_CONFIG_DIR takes precedence and the workdir is dash-encoded.
	want := filepath.Join("/cfg", "projects", "-Users-me-proj-cli-interactive-x", "abc-123.jsonl")
	if got := claudeTranscriptPath([]string{"CLAUDE_CONFIG_DIR=/cfg"}, "/Users/me/proj/cli_interactive-x", "abc-123"); got != want {
		t.Errorf("claudeTranscriptPath = %q, want %q", got, want)
	}

	// HOME fallback (<home>/.claude) when CLAUDE_CONFIG_DIR is absent.
	want2 := filepath.Join("/home/u", ".claude", "projects", "-a-b", "s.jsonl")
	if got := claudeTranscriptPath([]string{"HOME=/home/u"}, "/a/b", "s"); got != want2 {
		t.Errorf("claudeTranscriptPath(HOME) = %q, want %q", got, want2)
	}

	// Missing inputs → "".
	if got := claudeTranscriptPath(nil, "", "s"); got != "" {
		t.Errorf("claudeTranscriptPath(no workdir) = %q, want empty", got)
	}
}

func TestLastAssistantText(t *testing.T) {
	t.Parallel()
	cfg := t.TempDir()
	workDir := "/work/dir"
	sid := "sess-1"
	writeTranscript(t, cfg, workDir, sid,
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"working"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"API Error: 529 Overloaded. Try again."}]}}`,
		`{"type":"system","subtype":"turn_duration"}`,
	)
	env := []string{"CLAUDE_CONFIG_DIR=" + cfg}
	if got := lastAssistantText(env, workDir, sid); got != "API Error: 529 Overloaded. Try again." {
		t.Errorf("lastAssistantText = %q", got)
	}

	// Missing transcript → "".
	if got := lastAssistantText(env, "/no/such", "x"); got != "" {
		t.Errorf("lastAssistantText(missing) = %q, want empty", got)
	}
}

func TestClassifyExitOverloaded(t *testing.T) {
	t.Parallel()
	a := &ClaudeAdapter{}
	class, matched := a.ClassifyExit("API Error: 529 Overloaded. This is a server-side issue.", "", 1, nil, nil)
	if class != RetryClassRateLimit {
		t.Errorf("class = %v, want RetryClassRateLimit (matched=%q)", class, matched)
	}
}

func TestHandleInBandRateLimit_DetectsAndRetries(t *testing.T) {
	t.Parallel()
	cfg := t.TempDir()
	workDir := "/work/dir"
	sid := "sess-inband"
	writeTranscript(t, cfg, workDir, sid,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"API Error: 529 Overloaded."}]}}`,
	)

	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled → waitForRateLimitRetry returns immediately, no real sleep

	doneCh := make(chan struct{})
	close(doneCh)

	proc := newInbandProc(cfg, workDir, sid, clk)
	proc.doneCh = doneCh
	proc.cmd = &exec.Cmd{}

	if !s.handleInBandRateLimit(ctx, proc, SpawnRequest{}) {
		t.Fatal("handleInBandRateLimit = false, want true (529 in transcript)")
	}
	if proc.finalStatus != "CONTINUE" {
		t.Errorf("finalStatus = %q, want CONTINUE", proc.finalStatus)
	}
	if proc.rateLimitRetryCount != 1 {
		t.Errorf("rateLimitRetryCount = %d, want 1", proc.rateLimitRetryCount)
	}
}

func TestHandleInBandRateLimit_Skips(t *testing.T) {
	t.Parallel()
	cfg := t.TempDir()
	workDir := "/work/dir"

	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	// Normal final message → no match.
	writeTranscript(t, cfg, workDir, "sess-normal",
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"All done, tests pass."}]}}`)
	normal := newInbandProc(cfg, workDir, "sess-normal", clk)
	if s.handleInBandRateLimit(context.Background(), normal, SpawnRequest{}) {
		t.Error("normal final message should not trigger in-band retry")
	}

	// 529 present but agent not yet idle past the grace window.
	writeTranscript(t, cfg, workDir, "sess-busy",
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"API Error: 529 Overloaded."}]}}`)
	busy := newInbandProc(cfg, workDir, "sess-busy", clk)
	busy.lastMessageTime = clk.Now() // just spoke → not idle
	if s.handleInBandRateLimit(context.Background(), busy, SpawnRequest{}) {
		t.Error("non-idle agent should not trigger in-band retry")
	}

	// Rate-limit handling disabled.
	writeTranscript(t, cfg, workDir, "sess-off",
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"API Error: 529 Overloaded."}]}}`)
	off := newInbandProc(cfg, workDir, "sess-off", clk)
	off.rateLimitConfig.Enabled = false
	if s.handleInBandRateLimit(context.Background(), off, SpawnRequest{}) {
		t.Error("disabled rate-limit should not trigger in-band retry")
	}
}

// newInbandProc builds an idle, rate-limit-enabled Claude proc whose transcript
// lives under cfg.
func newInbandProc(cfg, workDir, sid string, clk clock.Clock) *processInfo {
	return &processInfo{
		adapter:         &ClaudeAdapter{},
		backend:         fakeBackend{name: "cli_interactive"},
		workDir:         workDir,
		sessionID:       sid,
		modelID:         "claude:opus-4-8-1m",
		env:             []string{"CLAUDE_CONFIG_DIR=" + cfg},
		lastMessageTime: clk.Now().Add(-2 * time.Minute),
		pendingMessages: make([]repo.MessageEntry, 0),
		rateLimitConfig: rateLimitConfig{Enabled: true, InitialBackoff: 60 * time.Second, MaxWait: 3600 * time.Second},
	}
}

func writeTranscript(t *testing.T, cfg, workDir, sid string, lines ...string) {
	t.Helper()
	dir := filepath.Join(cfg, "projects", nonAlnum.ReplaceAllString(workDir, "-"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir transcript dir: %v", err)
	}
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, sid+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
}
