package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBinaryBuild_NrworkflowCLI verifies that the CLI binary compiles successfully
func TestBinaryBuild_NrworkflowCLI(t *testing.T) {
	// Build to a temp location
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "nrworkflow")

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/nrworkflow")
	cmd.Dir = getBeDir(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrworkflow binary failed to compile: %v\nOutput: %s", err, output)
	}

	// Verify binary exists
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Fatalf("nrworkflow binary was not created at %s", binaryPath)
	}
}

// TestBinaryBuild_NrworkflowServer verifies that the server binary compiles successfully
func TestBinaryBuild_NrworkflowServer(t *testing.T) {
	// Build to a temp location
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "nrworkflow_server")

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/server")
	cmd.Dir = getBeDir(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrworkflow_server binary failed to compile: %v\nOutput: %s", err, output)
	}

	// Verify binary exists
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Fatalf("nrworkflow_server binary was not created at %s", binaryPath)
	}
}

// TestCLIBinary_Help verifies nrworkflow --help shows only CLI commands
func TestCLIBinary_Help(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildCLIBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrworkflow --help failed: %v\nOutput: %s", err, output)
	}

	helpText := string(output)

	// Extract the Available Commands section to avoid false positives from usage description
	availableCommandsStart := strings.Index(helpText, "Available Commands:")
	if availableCommandsStart == -1 {
		t.Fatal("nrworkflow --help missing 'Available Commands:' section")
	}
	availableCommandsSection := helpText[availableCommandsStart:]

	// Verify CLI commands are present
	requiredCommands := []string{"agent ", "findings ", "tickets ", "deps ", "version "}
	for _, cmdName := range requiredCommands {
		if !strings.Contains(availableCommandsSection, cmdName) {
			t.Errorf("nrworkflow Available Commands missing expected command %q\nOutput:\n%s", strings.TrimSpace(cmdName), helpText)
		}
	}

	// Verify serve command is NOT present in Available Commands
	if strings.Contains(availableCommandsSection, "serve ") {
		t.Errorf("nrworkflow Available Commands should NOT show 'serve' command\nOutput:\n%s", helpText)
	}
}

// TestCLIBinary_ServeCommandNotAvailable verifies nrworkflow serve returns unknown command error
func TestCLIBinary_ServeCommandNotAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildCLIBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "serve")
	output, err := cmd.CombinedOutput()

	// Should fail with unknown command error
	if err == nil {
		t.Errorf("nrworkflow serve should fail with unknown command error, but succeeded\nOutput: %s", output)
	}

	outputText := string(output)
	// Check for unknown command error message
	if !strings.Contains(outputText, "unknown command") && !strings.Contains(outputText, "Unknown command") {
		t.Errorf("nrworkflow serve should return 'unknown command' error\nOutput: %s", outputText)
	}
}

// TestServerBinary_Help verifies nrworkflow_server --help shows only server commands
func TestServerBinary_Help(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrworkflow_server --help failed: %v\nOutput: %s", err, output)
	}

	helpText := string(output)

	// Verify serve and version commands are present in Available Commands section
	if !strings.Contains(helpText, "serve") {
		t.Errorf("nrworkflow_server --help missing 'serve' command\nOutput:\n%s", helpText)
	}
	if !strings.Contains(helpText, "version") {
		t.Errorf("nrworkflow_server --help missing 'version' command\nOutput:\n%s", helpText)
	}

	// Verify CLI-only commands are NOT present in Available Commands section
	// Extract the Available Commands section to avoid false positives from usage description
	availableCommandsStart := strings.Index(helpText, "Available Commands:")
	if availableCommandsStart == -1 {
		t.Fatal("nrworkflow_server --help missing 'Available Commands:' section")
	}
	availableCommandsSection := helpText[availableCommandsStart:]

	forbiddenCommands := []string{"agent ", "findings ", "tickets ", "deps "}
	for _, cmdName := range forbiddenCommands {
		if strings.Contains(availableCommandsSection, cmdName) {
			t.Errorf("nrworkflow_server Available Commands should NOT show %q command\nOutput:\n%s", strings.TrimSpace(cmdName), helpText)
		}
	}
}

// TestServerBinary_TicketsCommandNotAvailable verifies nrworkflow_server tickets returns unknown command error
func TestServerBinary_TicketsCommandNotAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "tickets")
	output, err := cmd.CombinedOutput()

	// Should fail with unknown command error
	if err == nil {
		t.Errorf("nrworkflow_server tickets should fail with unknown command error, but succeeded\nOutput: %s", output)
	}

	outputText := string(output)
	// Check for unknown command error message
	if !strings.Contains(outputText, "unknown command") && !strings.Contains(outputText, "Unknown command") {
		t.Errorf("nrworkflow_server tickets should return 'unknown command' error\nOutput: %s", outputText)
	}
}

// TestServerBinary_AgentCommandNotAvailable verifies nrworkflow_server agent returns unknown command error
func TestServerBinary_AgentCommandNotAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "agent")
	output, err := cmd.CombinedOutput()

	// Should fail with unknown command error
	if err == nil {
		t.Errorf("nrworkflow_server agent should fail with unknown command error, but succeeded\nOutput: %s", output)
	}

	outputText := string(output)
	// Check for unknown command error message
	if !strings.Contains(outputText, "unknown command") && !strings.Contains(outputText, "Unknown command") {
		t.Errorf("nrworkflow_server agent should return 'unknown command' error\nOutput: %s", outputText)
	}
}

// TestCLIBinary_VersionCommand verifies nrworkflow version works
func TestCLIBinary_VersionCommand(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildCLIBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrworkflow version failed: %v\nOutput: %s", err, output)
	}

	// Should output version information (exact format may vary)
	outputText := string(output)
	if len(outputText) == 0 {
		t.Error("nrworkflow version returned empty output")
	}
}

// TestServerBinary_VersionCommand verifies nrworkflow_server version works
func TestServerBinary_VersionCommand(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrworkflow_server version failed: %v\nOutput: %s", err, output)
	}

	// Should output version information
	outputText := string(output)
	if len(outputText) == 0 {
		t.Error("nrworkflow_server version returned empty output")
	}
}

// TestCLIBinary_AgentSubcommands verifies nrworkflow agent subcommands exist
func TestCLIBinary_AgentSubcommands(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildCLIBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "agent", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrworkflow agent --help failed: %v\nOutput: %s", err, output)
	}

	helpText := string(output)
	expectedSubcommands := []string{"fail", "continue", "callback"}
	for _, subcmd := range expectedSubcommands {
		if !strings.Contains(helpText, subcmd) {
			t.Errorf("nrworkflow agent --help missing subcommand %q\nOutput:\n%s", subcmd, helpText)
		}
	}
}

// TestCLIBinary_FindingsSubcommands verifies nrworkflow findings subcommands exist
func TestCLIBinary_FindingsSubcommands(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildCLIBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "findings", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrworkflow findings --help failed: %v\nOutput: %s", err, output)
	}

	helpText := string(output)
	expectedSubcommands := []string{"add", "get", "append", "delete"}
	for _, subcmd := range expectedSubcommands {
		if !strings.Contains(helpText, subcmd) {
			t.Errorf("nrworkflow findings --help missing subcommand %q\nOutput:\n%s", subcmd, helpText)
		}
	}
}

// TestCLIBinary_TicketsSubcommands verifies nrworkflow tickets subcommands exist
func TestCLIBinary_TicketsSubcommands(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildCLIBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "tickets", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrworkflow tickets --help failed: %v\nOutput: %s", err, output)
	}

	helpText := string(output)
	expectedSubcommands := []string{"list", "get", "create", "update", "close", "reopen", "delete"}
	for _, subcmd := range expectedSubcommands {
		if !strings.Contains(helpText, subcmd) {
			t.Errorf("nrworkflow tickets --help missing subcommand %q\nOutput:\n%s", subcmd, helpText)
		}
	}
}

// TestCLIBinary_DepsSubcommands verifies nrworkflow deps subcommands exist
func TestCLIBinary_DepsSubcommands(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildCLIBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "deps", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrworkflow deps --help failed: %v\nOutput: %s", err, output)
	}

	helpText := string(output)
	expectedSubcommands := []string{"list", "add", "remove"}
	for _, subcmd := range expectedSubcommands {
		if !strings.Contains(helpText, subcmd) {
			t.Errorf("nrworkflow deps --help missing subcommand %q\nOutput:\n%s", subcmd, helpText)
		}
	}
}

// TestServerBinary_ServeCommandExists verifies nrworkflow_server serve command exists (but don't start server)
func TestServerBinary_ServeCommandExists(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	cmd := exec.Command(binaryPath, "serve", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nrworkflow_server serve --help failed: %v\nOutput: %s", err, output)
	}

	helpText := string(output)
	// Verify serve command help is shown
	if !strings.Contains(helpText, "serve") && !strings.Contains(helpText, "Start the nrworkflow server") {
		t.Errorf("nrworkflow_server serve --help did not show serve command help\nOutput:\n%s", helpText)
	}
}

// TestBinaryIndependence verifies binaries can be built independently without the other
func TestBinaryIndependence(t *testing.T) {
	beDir := getBeDir(t)
	tmpDir := t.TempDir()

	// Build CLI binary only
	cliBinaryPath := filepath.Join(tmpDir, "nrworkflow")
	cliCmd := exec.Command("go", "build", "-o", cliBinaryPath, "./cmd/nrworkflow")
	cliCmd.Dir = beDir
	if output, err := cliCmd.CombinedOutput(); err != nil {
		t.Fatalf("CLI binary build failed independently: %v\nOutput: %s", err, output)
	}

	// Build server binary only (in separate temp dir to ensure independence)
	tmpDir2 := t.TempDir()
	serverBinaryPath := filepath.Join(tmpDir2, "nrworkflow_server")
	serverCmd := exec.Command("go", "build", "-o", serverBinaryPath, "./cmd/server")
	serverCmd.Dir = beDir
	if output, err := serverCmd.CombinedOutput(); err != nil {
		t.Fatalf("Server binary build failed independently: %v\nOutput: %s", err, output)
	}

	// Verify both binaries exist and are different files
	cliInfo, err := os.Stat(cliBinaryPath)
	if err != nil {
		t.Fatalf("CLI binary not found: %v", err)
	}
	serverInfo, err := os.Stat(serverBinaryPath)
	if err != nil {
		t.Fatalf("Server binary not found: %v", err)
	}

	// Binaries should have different sizes (they register different commands)
	if cliInfo.Size() == serverInfo.Size() {
		t.Logf("Warning: CLI and server binaries have identical size (%d bytes), may indicate identical builds", cliInfo.Size())
	}
}

// TestMakefileTargets_Build verifies make build builds both binaries
func TestMakefileTargets_Build(t *testing.T) {
	beDir := getBeDir(t)

	// Clean first
	cleanCmd := exec.Command("make", "clean")
	cleanCmd.Dir = beDir
	if output, err := cleanCmd.CombinedOutput(); err != nil {
		t.Logf("make clean output: %s", output)
	}

	// Build both
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = beDir
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make build failed: %v\nOutput: %s", err, output)
	}

	// Verify both binaries exist
	cliBinary := filepath.Join(beDir, "nrworkflow")
	serverBinary := filepath.Join(beDir, "nrworkflow_server")

	if _, err := os.Stat(cliBinary); os.IsNotExist(err) {
		t.Errorf("make build did not create nrworkflow binary")
	}
	if _, err := os.Stat(serverBinary); os.IsNotExist(err) {
		t.Errorf("make build did not create nrworkflow_server binary")
	}

	// Clean up
	defer func() {
		cleanCmd := exec.Command("make", "clean")
		cleanCmd.Dir = beDir
		cleanCmd.Run()
	}()
}

// TestBinaryNaming verifies binary names match conventions
func TestBinaryNaming(t *testing.T) {
	// CLI binary should be named "nrworkflow"
	tmpDir := t.TempDir()
	cliBinary := buildCLIBinary(t, tmpDir)
	if !strings.HasSuffix(cliBinary, "nrworkflow") {
		t.Errorf("CLI binary name should be 'nrworkflow', got %s", filepath.Base(cliBinary))
	}

	// Server binary should be named "nrworkflow_server" (underscore, not hyphen)
	serverBinary := buildServerBinary(t, tmpDir)
	if !strings.HasSuffix(serverBinary, "nrworkflow_server") {
		t.Errorf("Server binary name should be 'nrworkflow_server', got %s", filepath.Base(serverBinary))
	}
}

// TestMakefileTargets_BuildCLI verifies make build-cli builds only the CLI binary
func TestMakefileTargets_BuildCLI(t *testing.T) {
	beDir := getBeDir(t)

	// Clean first
	cleanCmd := exec.Command("make", "clean")
	cleanCmd.Dir = beDir
	if output, err := cleanCmd.CombinedOutput(); err != nil {
		t.Logf("make clean output: %s", output)
	}

	// Build CLI only
	buildCmd := exec.Command("make", "build-cli")
	buildCmd.Dir = beDir
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make build-cli failed: %v\nOutput: %s", err, output)
	}

	// Verify CLI binary exists
	cliBinary := filepath.Join(beDir, "nrworkflow")
	if _, err := os.Stat(cliBinary); os.IsNotExist(err) {
		t.Errorf("make build-cli did not create nrworkflow binary")
	}

	// Clean up
	defer func() {
		cleanCmd := exec.Command("make", "clean")
		cleanCmd.Dir = beDir
		cleanCmd.Run()
	}()
}

// TestMakefileTargets_BuildServer verifies make build-server builds only the server binary
func TestMakefileTargets_BuildServer(t *testing.T) {
	beDir := getBeDir(t)

	// Clean first
	cleanCmd := exec.Command("make", "clean")
	cleanCmd.Dir = beDir
	if output, err := cleanCmd.CombinedOutput(); err != nil {
		t.Logf("make clean output: %s", output)
	}

	// Build server only
	buildCmd := exec.Command("make", "build-server")
	buildCmd.Dir = beDir
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make build-server failed: %v\nOutput: %s", err, output)
	}

	// Verify server binary exists
	serverBinary := filepath.Join(beDir, "nrworkflow_server")
	if _, err := os.Stat(serverBinary); os.IsNotExist(err) {
		t.Errorf("make build-server did not create nrworkflow_server binary")
	}

	// Clean up
	defer func() {
		cleanCmd := exec.Command("make", "clean")
		cleanCmd.Dir = beDir
		cleanCmd.Run()
	}()
}

// TestMakefileTargets_BuildRelease verifies make build-release builds both release binaries
func TestMakefileTargets_BuildRelease(t *testing.T) {
	beDir := getBeDir(t)

	// Clean first
	cleanCmd := exec.Command("make", "clean")
	cleanCmd.Dir = beDir
	if output, err := cleanCmd.CombinedOutput(); err != nil {
		t.Logf("make clean output: %s", output)
	}

	// Build release
	buildCmd := exec.Command("make", "build-release")
	buildCmd.Dir = beDir
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make build-release failed: %v\nOutput: %s", err, output)
	}

	// Verify both binaries exist
	cliBinary := filepath.Join(beDir, "nrworkflow")
	serverBinary := filepath.Join(beDir, "nrworkflow_server")

	if _, err := os.Stat(cliBinary); os.IsNotExist(err) {
		t.Errorf("make build-release did not create nrworkflow binary")
	}
	if _, err := os.Stat(serverBinary); os.IsNotExist(err) {
		t.Errorf("make build-release did not create nrworkflow_server binary")
	}

	// Verify binaries are executable
	if info, err := os.Stat(cliBinary); err == nil {
		if info.Mode()&0111 == 0 {
			t.Errorf("nrworkflow binary is not executable")
		}
	}
	if info, err := os.Stat(serverBinary); err == nil {
		if info.Mode()&0111 == 0 {
			t.Errorf("nrworkflow_server binary is not executable")
		}
	}

	// Clean up
	defer func() {
		cleanCmd := exec.Command("make", "clean")
		cleanCmd.Dir = beDir
		cleanCmd.Run()
	}()
}

// TestMakefileTargets_BuildCLIRelease verifies make build-cli-release builds stripped CLI binary
func TestMakefileTargets_BuildCLIRelease(t *testing.T) {
	beDir := getBeDir(t)

	// Clean first
	cleanCmd := exec.Command("make", "clean")
	cleanCmd.Dir = beDir
	if output, err := cleanCmd.CombinedOutput(); err != nil {
		t.Logf("make clean output: %s", output)
	}

	// Build CLI release
	buildCmd := exec.Command("make", "build-cli-release")
	buildCmd.Dir = beDir
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make build-cli-release failed: %v\nOutput: %s", err, output)
	}

	// Verify CLI binary exists
	cliBinary := filepath.Join(beDir, "nrworkflow")
	cliInfo, err := os.Stat(cliBinary)
	if os.IsNotExist(err) {
		t.Fatalf("make build-cli-release did not create nrworkflow binary")
	}

	// Build a debug version for size comparison
	tmpDir := t.TempDir()
	debugBinary := filepath.Join(tmpDir, "nrworkflow_debug")
	debugCmd := exec.Command("go", "build", "-o", debugBinary, "./cmd/nrworkflow")
	debugCmd.Dir = beDir
	if debugOutput, debugErr := debugCmd.CombinedOutput(); debugErr != nil {
		t.Logf("Debug build warning: %v\nOutput: %s", debugErr, debugOutput)
	}

	debugInfo, err := os.Stat(debugBinary)
	if err == nil {
		// Release binary should be smaller than debug binary (due to -ldflags="-s -w")
		if cliInfo.Size() >= debugInfo.Size() {
			t.Errorf("Release binary size (%d) should be smaller than debug binary (%d)", cliInfo.Size(), debugInfo.Size())
		}
	}

	// Clean up
	defer func() {
		cleanCmd := exec.Command("make", "clean")
		cleanCmd.Dir = beDir
		cleanCmd.Run()
	}()
}

// TestMakefileTargets_BuildServerRelease verifies make build-server-release builds stripped server binary
func TestMakefileTargets_BuildServerRelease(t *testing.T) {
	beDir := getBeDir(t)

	// Clean first
	cleanCmd := exec.Command("make", "clean")
	cleanCmd.Dir = beDir
	if output, err := cleanCmd.CombinedOutput(); err != nil {
		t.Logf("make clean output: %s", output)
	}

	// Build server release
	buildCmd := exec.Command("make", "build-server-release")
	buildCmd.Dir = beDir
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make build-server-release failed: %v\nOutput: %s", err, output)
	}

	// Verify server binary exists
	serverBinary := filepath.Join(beDir, "nrworkflow_server")
	serverInfo, err := os.Stat(serverBinary)
	if os.IsNotExist(err) {
		t.Fatalf("make build-server-release did not create nrworkflow_server binary")
	}

	// Build a debug version for size comparison
	tmpDir := t.TempDir()
	debugBinary := filepath.Join(tmpDir, "nrworkflow_server_debug")
	debugCmd := exec.Command("go", "build", "-o", debugBinary, "./cmd/server")
	debugCmd.Dir = beDir
	if debugOutput, debugErr := debugCmd.CombinedOutput(); debugErr != nil {
		t.Logf("Debug build warning: %v\nOutput: %s", debugErr, debugOutput)
	}

	debugInfo, err := os.Stat(debugBinary)
	if err == nil {
		// Release binary should be smaller than debug binary (due to -ldflags="-s -w")
		if serverInfo.Size() >= debugInfo.Size() {
			t.Errorf("Release binary size (%d) should be smaller than debug binary (%d)", serverInfo.Size(), debugInfo.Size())
		}
	}

	// Clean up
	defer func() {
		cleanCmd := exec.Command("make", "clean")
		cleanCmd.Dir = beDir
		cleanCmd.Run()
	}()
}

// TestMakefileTargets_Clean verifies make clean removes both binaries
func TestMakefileTargets_Clean(t *testing.T) {
	beDir := getBeDir(t)

	// Build both binaries first
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = beDir
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("make build failed: %v\nOutput: %s", err, output)
	}

	cliBinary := filepath.Join(beDir, "nrworkflow")
	serverBinary := filepath.Join(beDir, "nrworkflow_server")

	// Verify binaries exist before clean
	if _, err := os.Stat(cliBinary); os.IsNotExist(err) {
		t.Fatalf("nrworkflow binary should exist before clean")
	}
	if _, err := os.Stat(serverBinary); os.IsNotExist(err) {
		t.Fatalf("nrworkflow_server binary should exist before clean")
	}

	// Run clean
	cleanCmd := exec.Command("make", "clean")
	cleanCmd.Dir = beDir
	output, err := cleanCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make clean failed: %v\nOutput: %s", err, output)
	}

	// Verify both binaries are removed
	if _, err := os.Stat(cliBinary); err == nil {
		t.Errorf("nrworkflow binary should be removed by make clean")
	}
	if _, err := os.Stat(serverBinary); err == nil {
		t.Errorf("nrworkflow_server binary should be removed by make clean")
	}
}

// TestServerBinary_StartStop verifies server can start and gracefully stop
func TestServerBinary_StartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping server start test in short mode")
	}

	tmpDir := t.TempDir()
	binaryPath := buildServerBinary(t, tmpDir)

	// Use a temporary database and socket
	dbPath := filepath.Join(tmpDir, "test.db")
	socketPath := filepath.Join(tmpDir, "test.sock")

	cmd := exec.Command(binaryPath, "serve",
		"--data", dbPath,
		"--socket", socketPath,
		"--port", "0", // Use port 0 to let OS assign a free port
	)

	// Start server
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(500 * time.Millisecond)

	// Stop server gracefully
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Errorf("Failed to send interrupt signal: %v", err)
	}

	// Wait for server to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// Server exited (error is expected since we sent interrupt)
		t.Logf("Server exited with: %v", err)
	case <-time.After(5 * time.Second):
		// Server didn't exit, force kill
		cmd.Process.Kill()
		t.Error("Server did not exit gracefully within 5 seconds")
	}
}

// Helper functions

// getBeDir returns the absolute path to the be/ directory
func getBeDir(t *testing.T) string {
	t.Helper()
	// Start from test file location and navigate to be/
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// If we're already in be/cmd, go up one level
	if strings.HasSuffix(wd, "/be/cmd") || strings.HasSuffix(wd, "/be/cmd/nrworkflow") || strings.HasSuffix(wd, "/be/cmd/server") {
		return filepath.Join(wd, "..")
	}

	// If we're in be/, return as-is
	if strings.HasSuffix(wd, "/be") {
		return wd
	}

	// Otherwise assume we're in project root
	beDir := filepath.Join(wd, "be")
	if _, err := os.Stat(beDir); os.IsNotExist(err) {
		t.Fatalf("Cannot find be/ directory from %s", wd)
	}
	return beDir
}

// buildCLIBinary builds the CLI binary to tmpDir and returns the path
func buildCLIBinary(t *testing.T, tmpDir string) string {
	t.Helper()
	binaryPath := filepath.Join(tmpDir, "nrworkflow")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/nrworkflow")
	cmd.Dir = getBeDir(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build CLI binary: %v\nOutput: %s", err, output)
	}
	return binaryPath
}

// buildServerBinary builds the server binary to tmpDir and returns the path
func buildServerBinary(t *testing.T, tmpDir string) string {
	t.Helper()
	binaryPath := filepath.Join(tmpDir, "nrworkflow_server")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/server")
	cmd.Dir = getBeDir(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build server binary: %v\nOutput: %s", err, output)
	}
	return binaryPath
}
