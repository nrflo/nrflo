package spawner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseExternalMCPServersValidation(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
		wantLen int
	}{
		{name: "empty", raw: "", wantLen: 0},
		{name: "blank", raw: "  \n", wantLen: 0},
		{name: "stdio ok", raw: `{"unity":{"command":"uv","args":["run","server.py"],"env":{"A":"1"}}}`, wantLen: 1},
		{name: "http ok", raw: `{"docs":{"type":"http","url":"https://example.com/mcp"}}`, wantLen: 1},
		{name: "bad json", raw: `{`, wantErr: "invalid external_mcp_servers JSON"},
		{name: "unknown field", raw: `{"x":{"command":"a","commands":["b"]}}`, wantErr: "invalid external_mcp_servers JSON"},
		{name: "bad name", raw: `{"my server":{"command":"a"}}`, wantErr: "must match"},
		{name: "reserved name", raw: `{"NRFLO":{"command":"a"}}`, wantErr: "reserved"},
		{name: "stdio missing command", raw: `{"x":{"env":{"A":"1"}}}`, wantErr: "command is required"},
		{name: "http missing url", raw: `{"x":{"type":"sse"}}`, wantErr: "url is required"},
		{name: "unknown type", raw: `{"x":{"type":"grpc","command":"a"}}`, wantErr: "unknown type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			servers, err := ParseExternalMCPServers(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(servers) != tc.wantLen {
				t.Fatalf("want %d servers, got %d", tc.wantLen, len(servers))
			}
		})
	}
}

func TestBuildClaudeMCPConfigMergesExternalServers(t *testing.T) {
	external := map[string]ExternalMCPServer{
		"unity": {Command: "uv", Args: []string{"run", "server.py"}, Env: map[string]string{"PORT": "6400"}},
		"docs":  {Type: "http", URL: "https://example.com/mcp", Headers: map[string]string{"Authorization": "Bearer x"}},
	}
	cfg, allowed, err := buildClaudeMCPConfig(external)
	if err != nil {
		t.Fatalf("buildClaudeMCPConfig: %v", err)
	}
	for _, want := range []string{`"nrflo"`, `"unity"`, `"docs"`, `"agent","mcp"`, `"url":"https://example.com/mcp"`, `"PORT":"6400"`} {
		if !strings.Contains(cfg, want) {
			t.Errorf("mcp config missing %s: %s", want, cfg)
		}
	}
	if allowed != "mcp__nrflo__* mcp__docs__* mcp__unity__*" {
		t.Errorf("unexpected allowedTools: %q", allowed)
	}
}

func TestBuildClaudeMCPConfigNoExternal(t *testing.T) {
	cfg, allowed, err := buildClaudeMCPConfig(nil)
	if err != nil {
		t.Fatalf("buildClaudeMCPConfig: %v", err)
	}
	if allowed != "mcp__nrflo__*" {
		t.Errorf("unexpected allowedTools: %q", allowed)
	}
	if strings.Count(cfg, `"command"`) != 1 {
		t.Errorf("expected only the nrflo server: %s", cfg)
	}
}

func TestAppendExternalCodexMCPServers(t *testing.T) {
	dir := t.TempDir()
	seed := "[model]\nname = \"gpt\"\n\n[mcp_servers.unity]\ncommand = \"stale\"\n\n[mcp_servers.unity.env]\nOLD = \"1\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	servers := map[string]ExternalMCPServer{
		"unity": {Command: "uv", Args: []string{"run", "srv.py"}, Env: map[string]string{"PORT": "6400"}},
		"docs":  {Type: "http", URL: "https://example.com/mcp"},
	}
	if err := appendExternalCodexMCPServers(dir, servers); err != nil {
		t.Fatalf("appendExternalCodexMCPServers: %v", err)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	got := string(out)
	if strings.Contains(got, "stale") || strings.Contains(got, "OLD") {
		t.Errorf("same-name user table not stripped: %s", got)
	}
	if strings.Count(got, "[mcp_servers.unity]") != 1 {
		t.Errorf("expected exactly one unity table: %s", got)
	}
	for _, want := range []string{"[model]", `command = "uv"`, `args = ["run", "srv.py"]`, "[mcp_servers.unity.env]", `PORT = "6400"`} {
		if !strings.Contains(got, want) {
			t.Errorf("config.toml missing %s: %s", want, got)
		}
	}
	if strings.Contains(got, "docs") {
		t.Errorf("non-stdio server must be skipped for codex: %s", got)
	}
}
