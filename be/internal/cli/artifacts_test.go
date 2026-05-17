package cli

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/socket"
)

// startStubArtifactSocket starts a stub Unix socket that returns canned responses
// keyed by method name. Returns the socket path.
// Uses /tmp to keep the path short (macOS sun_path limit: 104 bytes).
func startStubArtifactSocket(t *testing.T, responses map[string]json.RawMessage) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "arttest")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sockPath := filepath.Join(dir, "t.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				scanner := bufio.NewScanner(conn)
				scanner.Buffer(make([]byte, 64*1024*1024), 64*1024*1024)
				for scanner.Scan() {
					var req socket.Request
					if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
						continue
					}
					result := responses[req.Method]
					resp := socket.Response{ID: req.ID, Result: result}
					data, _ := json.Marshal(resp)
					data = append(data, '\n')
					conn.Write(data)
				}
			}()
		}
	}()
	return sockPath
}

// setupCLIArtifactEnv sets env vars and package state for CLI artifact tests.
// Returns a cleanup function.
func setupCLIArtifactEnv(t *testing.T, sockPath string) {
	t.Helper()
	t.Setenv("NRFLO_SOCKET", sockPath)
	t.Setenv("NRFLO_PROJECT", "test-proj")
	t.Setenv("NRF_SESSION_ID", "test-sess")
	t.Setenv("NRF_WORKFLOW_INSTANCE_ID", "test-wfi")
	ProjectID = "test-proj"
	t.Cleanup(func() { ProjectID = "" })
}

// captureStdout redirects os.Stdout, runs fn, and returns the captured output.
// Must not be called from parallel tests (modifies os.Stdout).
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = orig
	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()
	return buf.String()
}

// --- Command Registration ---

func TestArtifactCmd_SubcommandsRegistered(t *testing.T) {
	t.Parallel()
	subcmds := getCommandNames(artifactCmd)
	for _, name := range []string{"add", "list", "get"} {
		if !contains(subcmds, name) {
			t.Errorf("artifactCmd missing subcommand %q", name)
		}
	}
}

func TestArtifactAddCmd_RequiresExactlyTwoArgs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args    []string
		wantErr bool
	}{
		{[]string{"file.txt", "NAME"}, false},
		{[]string{"file.txt"}, true},
		{[]string{"file.txt", "NAME", "extra"}, true},
		{[]string{}, true},
	} {
		err := artifactAddCmd.Args(artifactAddCmd, tc.args)
		if tc.wantErr && err == nil {
			t.Errorf("args=%v: expected error", tc.args)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("args=%v: unexpected error: %v", tc.args, err)
		}
	}
}

func TestArtifactGetCmd_RequiresExactlyOneArg(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args    []string
		wantErr bool
	}{
		{[]string{"NAME"}, false},
		{[]string{}, true},
		{[]string{"A", "B"}, true},
	} {
		err := artifactGetCmd.Args(artifactGetCmd, tc.args)
		if tc.wantErr && err == nil {
			t.Errorf("args=%v: expected error", tc.args)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("args=%v: unexpected error: %v", tc.args, err)
		}
	}
}

func TestArtifactListCmd_RequiresNoArgs(t *testing.T) {
	t.Parallel()
	if err := artifactListCmd.Args(artifactListCmd, []string{}); err != nil {
		t.Errorf("no args: unexpected error: %v", err)
	}
	if err := artifactListCmd.Args(artifactListCmd, []string{"extra"}); err == nil {
		t.Error("extra arg: expected error")
	}
}

// --- Output / Socket Interaction ---

func TestArtifactGetCmd_WritesPathNoNewline(t *testing.T) {
	// Not parallel: modifies os.Stdout and package-level ProjectID.
	sockPath := startStubArtifactSocket(t, map[string]json.RawMessage{
		"artifact.get": json.RawMessage(`{"path":"/tmp/materialized/myfile.txt"}`),
	})
	setupCLIArtifactEnv(t, sockPath)

	out := captureStdout(t, func() {
		if err := artifactGetCmd.RunE(artifactGetCmd, []string{"myfile.txt"}); err != nil {
			t.Errorf("RunE: %v", err)
		}
	})

	// Code: fmt.Print(result.Path) — no newline, no quotes
	if out != "/tmp/materialized/myfile.txt" {
		t.Errorf("output = %q, want %q", out, "/tmp/materialized/myfile.txt")
	}
}

func TestArtifactAddCmd_EncodesFileContent(t *testing.T) {
	// Not parallel: modifies os.Stdout and package-level ProjectID.

	// Capture the request sent to the socket to verify base64 encoding.
	// Use /tmp to keep the path short (macOS sun_path limit: 104 bytes).
	dir, err := os.MkdirTemp("/tmp", "artadd")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sockPath := filepath.Join(dir, "t.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	requestCh := make(chan socket.Request, 1)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				scanner := bufio.NewScanner(conn)
				for scanner.Scan() {
					var req socket.Request
					if err := json.Unmarshal(scanner.Bytes(), &req); err == nil {
						requestCh <- req
						resp := socket.Response{ID: req.ID, Result: json.RawMessage(`{"id":"aid","name":"data.bin"}`)}
						data, _ := json.Marshal(resp)
						conn.Write(append(data, '\n'))
					}
				}
			}()
		}
	}()

	fileContent := []byte("binary content here")
	filePath := filepath.Join(dir, "data.bin")
	os.WriteFile(filePath, fileContent, 0o644)

	setupCLIArtifactEnv(t, sockPath)

	captureStdout(t, func() {
		if err := artifactAddCmd.RunE(artifactAddCmd, []string{filePath, "data.bin"}); err != nil {
			t.Errorf("RunE: %v", err)
		}
	})

	// Verify the request sent to the socket has correct base64 content.
	req := <-requestCh
	var params struct {
		Name       string `json:"name"`
		ContentB64 string `json:"content_b64"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(params.ContentB64)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if !bytes.Equal(decoded, fileContent) {
		t.Errorf("decoded content = %q, want %q", decoded, fileContent)
	}
	if !strings.HasSuffix(params.Name, "data.bin") {
		t.Errorf("name = %q, want data.bin", params.Name)
	}
}
