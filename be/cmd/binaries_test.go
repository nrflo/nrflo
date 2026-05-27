package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBinaryBuild_NrflowServer verifies that the server binary compiles successfully
func TestBinaryBuild_NrflowServer(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "nrflo_server")

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/server")
	cmd.Dir = getBeDir(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrflo_server binary failed to compile: %v\nOutput: %s", err, output)
	}

	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Fatalf("nrflo_server binary was not created at %s", binaryPath)
	}
}

// TestServerBinary_Help verifies nrflo_server --help shows serve, version, and the
// agent infra parent, but no client commands.
func TestServerBinary_Help(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrflo_server --help failed: %v\nOutput: %s", err, output)
	}
	helpText := string(output)

	for _, want := range []string{"serve", "version", "agent"} {
		if !strings.Contains(helpText, want) {
			t.Errorf("nrflo_server --help missing %q\nOutput:\n%s", want, helpText)
		}
	}

	availableCommandsStart := strings.Index(helpText, "Available Commands:")
	if availableCommandsStart == -1 {
		t.Fatal("nrflo_server --help missing 'Available Commands:' section")
	}
	availableCommandsSection := helpText[availableCommandsStart:]
	for _, cmdName := range []string{"findings ", "tickets ", "deps "} {
		if strings.Contains(availableCommandsSection, cmdName) {
			t.Errorf("nrflo_server Available Commands should NOT show %q\nOutput:\n%s", strings.TrimSpace(cmdName), helpText)
		}
	}
}

// TestServerBinary_TicketsCommandNotAvailable verifies the removed client commands error out.
func TestServerBinary_TicketsCommandNotAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "tickets")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("nrflo_server tickets should fail with unknown command error, but succeeded\nOutput: %s", output)
	}
	if outputText := string(output); !strings.Contains(outputText, "unknown command") && !strings.Contains(outputText, "Unknown command") {
		t.Errorf("nrflo_server tickets should return 'unknown command' error\nOutput: %s", outputText)
	}
}

// TestServerBinary_AgentInfraSubcommands verifies nrflo_server hosts the agent
// infrastructure subcommands (MCP bridge + Claude hook forwarders) but not the
// agent-initiated commands (which are MCP tools, not CLI subcommands).
func TestServerBinary_AgentInfraSubcommands(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "agent", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrflo_server agent --help failed: %v\nOutput: %s", err, output)
	}
	helpText := string(output)
	availStart := strings.Index(helpText, "Available Commands:")
	if availStart == -1 {
		t.Fatalf("nrflo_server agent --help missing 'Available Commands:' section\nOutput:\n%s", helpText)
	}
	availSection := helpText[availStart:]
	for _, sub := range []string{"record-event", "statusline", "context-update", "mcp"} {
		if !strings.Contains(availSection, sub) {
			t.Errorf("nrflo_server agent --help missing infra subcommand %q\nOutput:\n%s", sub, helpText)
		}
	}
	for _, sub := range []string{"finished", "callback", "continue"} {
		if strings.Contains(availSection, sub) {
			t.Errorf("nrflo_server agent should NOT expose agent-initiated subcommand %q\nOutput:\n%s", sub, helpText)
		}
	}
}

// TestServerBinary_VersionCommand verifies nrflo_server version works
func TestServerBinary_VersionCommand(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrflo_server version failed: %v\nOutput: %s", err, output)
	}
	if len(string(output)) == 0 {
		t.Error("nrflo_server version returned empty output")
	}
}

// TestServerBinary_ServeCommandExists verifies nrflo_server serve command exists.
func TestServerBinary_ServeCommandExists(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "serve", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrflo_server serve --help failed: %v\nOutput: %s", err, output)
	}
	helpText := string(output)
	if !strings.Contains(helpText, "serve") && !strings.Contains(helpText, "Start the nrflo server") {
		t.Errorf("nrflo_server serve --help did not show serve command help\nOutput:\n%s", helpText)
	}
}

// TestBinaryNaming verifies the server binary name matches convention.
func TestBinaryNaming(t *testing.T) {
	tmpDir := t.TempDir()
	serverBinary := buildServerBinary(t, tmpDir)
	if !strings.HasSuffix(serverBinary, "nrflo_server") {
		t.Errorf("Server binary name should be 'nrflo_server', got %s", filepath.Base(serverBinary))
	}
}

// TestMakefileTargets_Build verifies make build builds the (single) server binary.
func TestMakefileTargets_Build(t *testing.T) {
	beDir := getBeDir(t)
	runMake(t, beDir, "clean")
	defer runMake(t, beDir, "clean")

	if output, err := makeCmd(beDir, "build").CombinedOutput(); err != nil {
		t.Fatalf("make build failed: %v\nOutput: %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(beDir, "nrflo_server")); os.IsNotExist(err) {
		t.Errorf("make build did not create nrflo_server binary")
	}
	// The nrflo CLI binary has been removed.
	if _, err := os.Stat(filepath.Join(beDir, "nrflo")); err == nil {
		t.Errorf("make build should NOT create a nrflo CLI binary (removed)")
	}
}

// TestMakefileTargets_BuildServer verifies make build-server builds the server binary.
func TestMakefileTargets_BuildServer(t *testing.T) {
	beDir := getBeDir(t)
	runMake(t, beDir, "clean")
	defer runMake(t, beDir, "clean")

	if output, err := makeCmd(beDir, "build-server").CombinedOutput(); err != nil {
		t.Fatalf("make build-server failed: %v\nOutput: %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(beDir, "nrflo_server")); os.IsNotExist(err) {
		t.Errorf("make build-server did not create nrflo_server binary")
	}
}

// TestMakefileTargets_BuildRelease verifies make build-release builds the server binary.
func TestMakefileTargets_BuildRelease(t *testing.T) {
	beDir := getBeDir(t)
	runMake(t, beDir, "clean")
	defer runMake(t, beDir, "clean")

	if output, err := makeCmd(beDir, "build-release").CombinedOutput(); err != nil {
		t.Fatalf("make build-release failed: %v\nOutput: %s", err, output)
	}
	serverBinary := filepath.Join(beDir, "nrflo_server")
	info, err := os.Stat(serverBinary)
	if os.IsNotExist(err) {
		t.Fatalf("make build-release did not create nrflo_server binary")
	}
	if err == nil && info.Mode()&0111 == 0 {
		t.Errorf("nrflo_server binary is not executable")
	}
}

// TestMakefileTargets_BuildServerRelease verifies make build-release-server builds a stripped binary.
func TestMakefileTargets_BuildServerRelease(t *testing.T) {
	beDir := getBeDir(t)
	runMake(t, beDir, "clean")
	defer runMake(t, beDir, "clean")

	if output, err := makeCmd(beDir, "build-release-server").CombinedOutput(); err != nil {
		t.Fatalf("make build-release-server failed: %v\nOutput: %s", err, output)
	}
	serverBinary := filepath.Join(beDir, "nrflo_server")
	serverInfo, err := os.Stat(serverBinary)
	if os.IsNotExist(err) {
		t.Fatalf("make build-release-server did not create nrflo_server binary")
	}

	tmpDir := t.TempDir()
	debugBinary := filepath.Join(tmpDir, "nrflo_server_debug")
	debugCmd := exec.Command("go", "build", "-o", debugBinary, "./cmd/server")
	debugCmd.Dir = beDir
	if debugOutput, debugErr := debugCmd.CombinedOutput(); debugErr != nil {
		t.Logf("Debug build warning: %v\nOutput: %s", debugErr, debugOutput)
	}
	if debugInfo, statErr := os.Stat(debugBinary); statErr == nil {
		if serverInfo.Size() >= debugInfo.Size() {
			t.Errorf("Release binary size (%d) should be smaller than debug binary (%d)", serverInfo.Size(), debugInfo.Size())
		}
	}
}

// TestMakefileTargets_Clean verifies make clean removes the server binary.
func TestMakefileTargets_Clean(t *testing.T) {
	beDir := getBeDir(t)

	if output, err := makeCmd(beDir, "build").CombinedOutput(); err != nil {
		t.Fatalf("make build failed: %v\nOutput: %s", err, output)
	}
	serverBinary := filepath.Join(beDir, "nrflo_server")
	if _, err := os.Stat(serverBinary); os.IsNotExist(err) {
		t.Fatalf("nrflo_server binary should exist before clean")
	}

	if output, err := makeCmd(beDir, "clean").CombinedOutput(); err != nil {
		t.Fatalf("make clean failed: %v\nOutput: %s", err, output)
	}
	if _, err := os.Stat(serverBinary); err == nil {
		t.Errorf("nrflo_server binary should be removed by make clean")
	}
}

// TestServerBinary_StartStop verifies server can start and gracefully stop
func TestServerBinary_StartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping server start test in short mode")
	}

	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	socketPath := filepath.Join(tmpDir, "test.sock")

	cmd := exec.Command(binaryPath, "serve",
		"--data", dbPath,
		"--socket", socketPath,
		"--port", "0",
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Errorf("Failed to send interrupt signal: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		t.Logf("Server exited with: %v", err)
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Error("Server did not exit gracefully within 5 seconds")
	}
}

// Helper functions

func makeCmd(beDir, target string) *exec.Cmd {
	cmd := exec.Command("make", target)
	cmd.Dir = beDir
	return cmd
}

func runMake(t *testing.T, beDir, target string) {
	t.Helper()
	if output, err := makeCmd(beDir, target).CombinedOutput(); err != nil {
		t.Logf("make %s output: %s", target, output)
	}
}

// getBeDir returns the absolute path to the be/ directory
func getBeDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	if strings.HasSuffix(wd, "/be/cmd") || strings.HasSuffix(wd, "/be/cmd/server") {
		return filepath.Join(wd, "..")
	}
	if strings.HasSuffix(wd, "/be") {
		return wd
	}
	beDir := filepath.Join(wd, "be")
	if _, err := os.Stat(beDir); os.IsNotExist(err) {
		t.Fatalf("Cannot find be/ directory from %s", wd)
	}
	return beDir
}

// buildServerBinary builds the server binary to tmpDir and returns the path
func buildServerBinary(t *testing.T, tmpDir string) string {
	t.Helper()
	binaryPath := filepath.Join(tmpDir, "nrflo_server")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/server")
	cmd.Dir = getBeDir(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build server binary: %v\nOutput: %s", err, output)
	}
	return binaryPath
}
