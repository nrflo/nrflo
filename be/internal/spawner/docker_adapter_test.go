package spawner

import (
	"os/exec"
	"strings"
	"testing"
)

// ---- mock adapter -------------------------------------------------------

type mockCLIAdapter struct {
	name                     string
	mapModelResult           string
	supportsSessionID        bool
	supportsSystemPromptFile bool
	supportsResume           bool
	usesStdinPrompt          bool
	// Determines what BuildCommand / BuildResumeCommand return.
	// A non-empty string → a minimal *exec.Cmd; empty string for resume → nil.
	buildCmdPath       string
	buildResumeCmdPath string // empty means BuildResumeCommand returns nil
}

func (m *mockCLIAdapter) Name() string                    { return m.name }
func (m *mockCLIAdapter) MapModel(model string) string    { return m.mapModelResult }
func (m *mockCLIAdapter) SupportsSessionID() bool         { return m.supportsSessionID }
func (m *mockCLIAdapter) SupportsSystemPromptFile() bool  { return m.supportsSystemPromptFile }
func (m *mockCLIAdapter) SupportsResume() bool            { return m.supportsResume }
func (m *mockCLIAdapter) UsesStdinPrompt() bool           { return m.usesStdinPrompt }

func (m *mockCLIAdapter) BuildCommand(opts SpawnOptions) *exec.Cmd {
	return mockExecCmd(m.buildCmdPath, "cmd-arg1", "cmd-arg2")
}

func (m *mockCLIAdapter) BuildResumeCommand(opts ResumeOptions) *exec.Cmd {
	if m.buildResumeCmdPath == "" {
		return nil
	}
	return mockExecCmd(m.buildResumeCmdPath, "rsm-arg1")
}

// mockExecCmd builds a bare *exec.Cmd without touching the filesystem.
func mockExecCmd(path string, args ...string) *exec.Cmd {
	allArgs := append([]string{path}, args...)
	return &exec.Cmd{
		Path: path,
		Args: allArgs,
		Env:  []string{"INNER_ENV=inner"},
	}
}

// defaultMock returns a mock with sensible defaults for building a spawn command.
func defaultMock() *mockCLIAdapter {
	return &mockCLIAdapter{
		name:                     "claude",
		mapModelResult:           "claude-opus-4-5",
		supportsSessionID:        true,
		supportsSystemPromptFile: true,
		supportsResume:           true,
		usesStdinPrompt:          false,
		buildCmdPath:             "/usr/local/bin/claude",
		buildResumeCmdPath:       "/usr/local/bin/claude",
	}
}

func defaultConfig() DockerConfig {
	return DockerConfig{
		ProjectRoot: "/projects/myapp",
		HomeDir:     "/home/user",
		UID:         1000,
		GID:         1001,
	}
}

// ---- delegation tests ---------------------------------------------------

func TestDockerCLIAdapter_DelegatesMethods(t *testing.T) {
	t.Helper() // not a helper, but shows the pattern; real helpers below

	mock := &mockCLIAdapter{
		name:                     "opencode",
		mapModelResult:           "openai/gpt-5.2",
		supportsSessionID:        false,
		supportsSystemPromptFile: false,
		supportsResume:           false,
		usesStdinPrompt:          true,
		buildCmdPath:             "/usr/bin/opencode",
	}
	adapter := NewDockerCLIAdapter(mock, defaultConfig())

	if got := adapter.Name(); got != "opencode" {
		t.Errorf("Name() = %q, want %q", got, "opencode")
	}
	if got := adapter.MapModel("gpt_high"); got != "openai/gpt-5.2" {
		t.Errorf("MapModel() = %q, want %q", got, "openai/gpt-5.2")
	}
	if adapter.SupportsSessionID() {
		t.Error("SupportsSessionID() = true, want false")
	}
	if adapter.SupportsSystemPromptFile() {
		t.Error("SupportsSystemPromptFile() = true, want false")
	}
	if adapter.SupportsResume() {
		t.Error("SupportsResume() = true, want false")
	}
	if !adapter.UsesStdinPrompt() {
		t.Error("UsesStdinPrompt() = false, want true")
	}
}

// ---- BuildCommand: outer structure --------------------------------------

func TestDockerCLIAdapter_BuildCommand_UsesDockerBinary(t *testing.T) {
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "sess1", Env: nil})

	if !strings.HasSuffix(cmd.Path, "docker") {
		t.Errorf("cmd.Path = %q, want path ending in 'docker'", cmd.Path)
	}
	if cmd.Args[0] != "docker" && !strings.HasSuffix(cmd.Args[0], "docker") {
		t.Errorf("cmd.Args[0] = %q, want 'docker'", cmd.Args[0])
	}
}

func TestDockerCLIAdapter_BuildCommand_RunFlags(t *testing.T) {
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "sess1"})
	args := strings.Join(cmd.Args, " ")

	for _, flag := range []string{"run", "--rm", "--platform", "linux/arm64"} {
		if !strings.Contains(args, flag) {
			t.Errorf("args missing %q: %s", flag, args)
		}
	}
}

// ---- Container naming ---------------------------------------------------

func TestDockerCLIAdapter_ContainerName_Short(t *testing.T) {
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "short"})
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "--name nrwf-short") {
		t.Errorf("args missing container name 'nrwf-short': %s", args)
	}
}

func TestDockerCLIAdapter_ContainerName_TruncateTo12(t *testing.T) {
	sessionID := "abcdefghijklmnopqrstuvwxyz" // 26 chars
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: sessionID})
	args := strings.Join(cmd.Args, " ")

	expected := "--name nrwf-abcdefghijkl" // first 12 chars
	if !strings.Contains(args, expected) {
		t.Errorf("args missing truncated container name %q: %s", expected, args)
	}

	// Ensure full session ID is NOT in name
	if strings.Contains(args, "--name nrwf-"+sessionID) {
		t.Errorf("container name should be truncated to 12 chars but got full ID: %s", args)
	}
}

func TestDockerCLIAdapter_ContainerName_ExactlyTwelve(t *testing.T) {
	sessionID := "123456789012" // exactly 12 chars
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: sessionID})
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "--name nrwf-123456789012") {
		t.Errorf("args missing container name: %s", args)
	}
}

// ---- Resume container naming --------------------------------------------

func TestDockerCLIAdapter_BuildResumeCommand_ContainerNameSuffix(t *testing.T) {
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildResumeCommand(ResumeOptions{SessionID: "sess1"})
	if cmd == nil {
		t.Fatal("BuildResumeCommand returned nil, want non-nil")
	}
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "--name nrwf-sess1-rsm") {
		t.Errorf("resume container name should end in -rsm: %s", args)
	}
}

func TestDockerCLIAdapter_BuildResumeCommand_TruncatesSessionID(t *testing.T) {
	sessionID := "abcdefghijklmnopqrstuvwxyz"
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildResumeCommand(ResumeOptions{SessionID: sessionID})
	if cmd == nil {
		t.Fatal("BuildResumeCommand returned nil")
	}
	args := strings.Join(cmd.Args, " ")

	expected := "--name nrwf-abcdefghijkl-rsm"
	if !strings.Contains(args, expected) {
		t.Errorf("args missing truncated resume container name %q: %s", expected, args)
	}
}

// ---- BuildResumeCommand: inner returns nil --------------------------------

func TestDockerCLIAdapter_BuildResumeCommand_NilWhenInnerNil(t *testing.T) {
	mock := defaultMock()
	mock.buildResumeCmdPath = "" // inner returns nil
	adapter := NewDockerCLIAdapter(mock, defaultConfig())

	cmd := adapter.BuildResumeCommand(ResumeOptions{SessionID: "sess1"})
	if cmd != nil {
		t.Errorf("BuildResumeCommand() = %v, want nil when inner returns nil", cmd)
	}
}

// ---- Environment forwarding ---------------------------------------------

func TestDockerCLIAdapter_BuildCommand_HostUIDGID(t *testing.T) {
	cfg := defaultConfig()
	cfg.UID = 1234
	cfg.GID = 5678
	adapter := NewDockerCLIAdapter(defaultMock(), cfg)
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "-e HOST_UID=1234") {
		t.Errorf("args missing HOST_UID: %s", args)
	}
	if !strings.Contains(args, "-e HOST_GID=5678") {
		t.Errorf("args missing HOST_GID: %s", args)
	}
}

func TestDockerCLIAdapter_BuildCommand_ForwardsEnvVars(t *testing.T) {
	envVars := []string{
		"NRWORKFLOW_PROJECT=myproj",
		"NRWF_SPAWNED=1",
		"NRWF_CONTEXT_THRESHOLD=25",
	}
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s", Env: envVars})
	args := strings.Join(cmd.Args, " ")

	for _, e := range envVars {
		if !strings.Contains(args, "-e "+e) {
			t.Errorf("args missing forwarded env var %q: %s", e, args)
		}
	}
}

func TestDockerCLIAdapter_BuildCommand_CopiesInnerCmdEnv(t *testing.T) {
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})

	// The mock sets Env: []string{"INNER_ENV=inner"}
	found := false
	for _, e := range cmd.Env {
		if e == "INNER_ENV=inner" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("BuildCommand did not copy inner cmd.Env; got %v", cmd.Env)
	}
}

// ---- Volume mounts ------------------------------------------------------

func assertMount(t *testing.T, args, mount string) {
	t.Helper()
	if !strings.Contains(args, "-v "+mount) {
		t.Errorf("args missing mount %q: %s", "-v "+mount, args)
	}
}

func TestDockerCLIAdapter_VolumeMounts_HomeDirectories(t *testing.T) {
	cfg := DockerConfig{
		ProjectRoot: "/projects/app",
		HomeDir:     "/home/alice",
		UID:         1000,
		GID:         1000,
	}
	adapter := NewDockerCLIAdapter(defaultMock(), cfg)
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})
	args := strings.Join(cmd.Args, " ")

	assertMount(t, args, "/home/alice/.claude:/home/alice/.claude")
	assertMount(t, args, "/home/alice/.config/opencode:/home/alice/.config/opencode")
	assertMount(t, args, "/home/alice/.local/share/opencode:/home/alice/.local/share/opencode")
	assertMount(t, args, "/home/alice/.ai_common/safety.json:/home/alice/.ai_common/safety.json:ro")
}

func TestDockerCLIAdapter_VolumeMounts_SharedTmp(t *testing.T) {
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})
	args := strings.Join(cmd.Args, " ")

	assertMount(t, args, "/tmp/nrworkflow:/tmp/nrworkflow")
	assertMount(t, args, "/tmp/usable_context.json:/tmp/usable_context.json")
}

func TestDockerCLIAdapter_VolumeMounts_DirectProjectMount(t *testing.T) {
	cfg := DockerConfig{
		ProjectRoot:  "/projects/app",
		WorktreePath: "", // no worktree
		HomeDir:      "/home/user",
	}
	adapter := NewDockerCLIAdapter(defaultMock(), cfg)
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})
	args := strings.Join(cmd.Args, " ")

	// Without worktree: project root is mounted at itself
	assertMount(t, args, "/projects/app:/projects/app")
}

func TestDockerCLIAdapter_VolumeMounts_WorktreeMount(t *testing.T) {
	cfg := DockerConfig{
		ProjectRoot:  "/projects/app",
		WorktreePath: "/tmp/nrworkflow/worktrees/ticket-123",
		HomeDir:      "/home/user",
	}
	adapter := NewDockerCLIAdapter(defaultMock(), cfg)
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})
	args := strings.Join(cmd.Args, " ")

	// With worktree: worktree is mounted AT the original project root path
	assertMount(t, args, "/tmp/nrworkflow/worktrees/ticket-123:/projects/app")

	// Ensure the direct project mount is NOT used
	if strings.Contains(args, "-v /projects/app:/projects/app") {
		t.Errorf("should not have direct project mount when worktree is set: %s", args)
	}
}

// ---- Working directory --------------------------------------------------

func TestDockerCLIAdapter_WorkingDir_WithoutWorktree(t *testing.T) {
	cfg := DockerConfig{
		ProjectRoot:  "/projects/app",
		WorktreePath: "",
		HomeDir:      "/home/user",
	}
	adapter := NewDockerCLIAdapter(defaultMock(), cfg)
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})

	if cmd.Dir != "/projects/app" {
		t.Errorf("cmd.Dir = %q, want /projects/app", cmd.Dir)
	}
}

func TestDockerCLIAdapter_WorkingDir_WithWorktree(t *testing.T) {
	cfg := DockerConfig{
		ProjectRoot:  "/projects/app",
		WorktreePath: "/tmp/nrworkflow/worktrees/ticket-123",
		HomeDir:      "/home/user",
	}
	adapter := NewDockerCLIAdapter(defaultMock(), cfg)
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})

	if cmd.Dir != "/tmp/nrworkflow/worktrees/ticket-123" {
		t.Errorf("cmd.Dir = %q, want /tmp/nrworkflow/worktrees/ticket-123", cmd.Dir)
	}
}

// ---- Container working directory inside (-w flag) ----------------------

func TestDockerCLIAdapter_ContainerWorkDir(t *testing.T) {
	cfg := DockerConfig{
		ProjectRoot: "/projects/myapp",
		HomeDir:     "/home/user",
	}
	adapter := NewDockerCLIAdapter(defaultMock(), cfg)
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "-w /projects/myapp") {
		t.Errorf("args missing container working dir flag: %s", args)
	}
}

// ---- Image name ---------------------------------------------------------

func TestDockerCLIAdapter_ImageName(t *testing.T) {
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "nrworkflow-agent") {
		t.Errorf("args missing image name 'nrworkflow-agent': %s", args)
	}
}

// ---- Inner command appended after image name ---------------------------

func TestDockerCLIAdapter_InnerCommandAppenedAfterImage(t *testing.T) {
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})

	// Locate "nrworkflow-agent" in args, then confirm inner binary follows
	imageIdx := -1
	for i, a := range cmd.Args {
		if a == "nrworkflow-agent" {
			imageIdx = i
			break
		}
	}
	if imageIdx < 0 {
		t.Fatalf("'nrworkflow-agent' not found in args: %v", cmd.Args)
	}
	if imageIdx+1 >= len(cmd.Args) {
		t.Fatalf("no args after image name; expected inner command")
	}
	// The next arg should be the inner binary path
	if cmd.Args[imageIdx+1] != "/usr/local/bin/claude" {
		t.Errorf("args after image = %q, want /usr/local/bin/claude", cmd.Args[imageIdx+1])
	}
}

// ---- Resume inner args propagation ------------------------------------

func TestDockerCLIAdapter_BuildResumeCommand_PropagatesInnerArgs(t *testing.T) {
	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	cmd := adapter.BuildResumeCommand(ResumeOptions{SessionID: "sess99"})
	if cmd == nil {
		t.Fatal("BuildResumeCommand returned nil")
	}

	// Inner command has args: [/usr/local/bin/claude, rsm-arg1]
	imageIdx := -1
	for i, a := range cmd.Args {
		if a == "nrworkflow-agent" {
			imageIdx = i
			break
		}
	}
	if imageIdx < 0 {
		t.Fatalf("'nrworkflow-agent' not found in args: %v", cmd.Args)
	}
	if imageIdx+1 >= len(cmd.Args) {
		t.Fatalf("no args after image name in resume command")
	}
	if cmd.Args[imageIdx+1] != "/usr/local/bin/claude" {
		t.Errorf("inner binary after image = %q, want /usr/local/bin/claude", cmd.Args[imageIdx+1])
	}
}

// ---- Table-driven: containerName helper ---------------------------------

func TestDockerCLIAdapter_ContainerNameTable(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		isResume  bool
		want      string
	}{
		{"short id, spawn", "abc", false, "nrwf-abc"},
		{"short id, resume", "abc", true, "nrwf-abc-rsm"},
		{"exactly 12, spawn", "123456789012", false, "nrwf-123456789012"},
		{"exactly 12, resume", "123456789012", true, "nrwf-123456789012-rsm"},
		{"long id truncated, spawn", "abcdefghijklmnop", false, "nrwf-abcdefghijkl"},
		{"long id truncated, resume", "abcdefghijklmnop", true, "nrwf-abcdefghijkl-rsm"},
		{"empty id", "", false, "nrwf-"},
	}

	adapter := NewDockerCLIAdapter(defaultMock(), defaultConfig())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.containerName(tt.sessionID, tt.isResume)
			if got != tt.want {
				t.Errorf("containerName(%q, %v) = %q, want %q", tt.sessionID, tt.isResume, got, tt.want)
			}
		})
	}
}

// ---- Table-driven: hostDir helper --------------------------------------

func TestDockerCLIAdapter_HostDirTable(t *testing.T) {
	tests := []struct {
		name         string
		projectRoot  string
		worktreePath string
		wantDir      string
	}{
		{"no worktree", "/projects/app", "", "/projects/app"},
		{"with worktree", "/projects/app", "/tmp/wt/ticket-1", "/tmp/wt/ticket-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DockerConfig{
				ProjectRoot:  tt.projectRoot,
				WorktreePath: tt.worktreePath,
				HomeDir:      "/home/user",
			}
			adapter := NewDockerCLIAdapter(defaultMock(), cfg)
			got := adapter.hostDir()
			if got != tt.wantDir {
				t.Errorf("hostDir() = %q, want %q", got, tt.wantDir)
			}
		})
	}
}

// ---- Implements CLIAdapter interface -----------------------------------

func TestDockerCLIAdapter_ImplementsCLIAdapter(t *testing.T) {
	var _ CLIAdapter = NewDockerCLIAdapter(defaultMock(), defaultConfig())
}

// ---- Safety.json is read-only ------------------------------------------

func TestDockerCLIAdapter_SafetyJsonMountedReadOnly(t *testing.T) {
	cfg := DockerConfig{
		ProjectRoot: "/projects/app",
		HomeDir:     "/home/user",
	}
	adapter := NewDockerCLIAdapter(defaultMock(), cfg)
	cmd := adapter.BuildCommand(SpawnOptions{SessionID: "s"})
	args := strings.Join(cmd.Args, " ")

	expected := "/home/user/.ai_common/safety.json:/home/user/.ai_common/safety.json:ro"
	if !strings.Contains(args, expected) {
		t.Errorf("safety.json should be mounted :ro; missing %q in: %s", expected, args)
	}
}
