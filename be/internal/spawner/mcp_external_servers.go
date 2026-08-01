package spawner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ExternalMCPServer is one project-configured MCP server passed through to
// spawned CLI agents alongside the nrflo bridge. Shape mirrors Claude's
// --mcp-config server entries: stdio servers use command/args/env, http/sse
// servers use url/headers.
type ExternalMCPServer struct {
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

var externalMCPNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ParseExternalMCPServers validates the external_mcp_servers project config
// JSON: a map of server name to server spec. Empty input yields nil. Names
// must be MCP-tool-pattern safe and not shadow the nrflo bridge; stdio
// servers require command, http/sse require url.
func ParseExternalMCPServers(raw string) (map[string]ExternalMCPServer, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var servers map[string]ExternalMCPServer
	if err := dec.Decode(&servers); err != nil {
		return nil, fmt.Errorf("invalid external_mcp_servers JSON: %w", err)
	}
	for name, srv := range servers {
		if !externalMCPNameRe.MatchString(name) {
			return nil, fmt.Errorf("server name %q: must match %s", name, externalMCPNameRe.String())
		}
		if strings.EqualFold(name, "nrflo") {
			return nil, fmt.Errorf("server name %q is reserved", name)
		}
		switch srv.Type {
		case "", "stdio":
			if srv.Command == "" {
				return nil, fmt.Errorf("server %q: command is required for stdio servers", name)
			}
		case "http", "sse":
			if srv.URL == "" {
				return nil, fmt.Errorf("server %q: url is required for type %q", name, srv.Type)
			}
		default:
			return nil, fmt.Errorf("server %q: unknown type %q (stdio, http, sse)", name, srv.Type)
		}
	}
	return servers, nil
}

func sortedExternalMCPNames(servers map[string]ExternalMCPServer) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// buildClaudeMCPConfig returns the --mcp-config JSON registering the nrflo
// agent mcp bridge plus any project-configured external servers, and the
// matching space-separated --allowedTools value (mcp__<name>__* per server).
func buildClaudeMCPConfig(external map[string]ExternalMCPServer) (mcpConfigJSON, allowedTools string, err error) {
	servers := map[string]interface{}{
		"nrflo": map[string]interface{}{
			"command": resolvedNrfloPath(),
			"args":    []string{"agent", "mcp"},
		},
	}
	allowed := []string{"mcp__nrflo__*"}
	for _, name := range sortedExternalMCPNames(external) {
		servers[name] = external[name]
		allowed = append(allowed, "mcp__"+name+"__*")
	}
	cfg, err := json.Marshal(map[string]interface{}{"mcpServers": servers})
	if err != nil {
		return "", "", err
	}
	return string(cfg), strings.Join(allowed, " "), nil
}

// appendExternalCodexMCPServers appends an [mcp_servers.<name>] table per
// stdio external server to the per-session CODEX_HOME/config.toml. Same-name
// tables inherited from the user's config are stripped first — the app-server
// rejects duplicate keys. Non-stdio (http/sse) servers are skipped: codex
// config only takes command-launched MCP servers.
func appendExternalCodexMCPServers(dir string, servers map[string]ExternalMCPServer) error {
	if len(servers) == 0 {
		return nil
	}
	path := filepath.Join(dir, "config.toml")
	var stripPrefixes []string
	for _, name := range sortedExternalMCPNames(servers) {
		stripPrefixes = append(stripPrefixes, "[mcp_servers."+name+"]", "[mcp_servers."+name+".")
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out := stripTOMLTables(existing, stripPrefixes)

	var b strings.Builder
	b.Write(out)
	for _, name := range sortedExternalMCPNames(servers) {
		srv := servers[name]
		if srv.Type != "" && srv.Type != "stdio" {
			continue
		}
		quotedArgs := make([]string, len(srv.Args))
		for i, a := range srv.Args {
			quotedArgs[i] = fmt.Sprintf("%q", a)
		}
		fmt.Fprintf(&b, "\n[mcp_servers.%s]\ncommand = %q\nargs = [%s]\n", name, srv.Command, strings.Join(quotedArgs, ", "))
		if len(srv.Env) > 0 {
			fmt.Fprintf(&b, "\n[mcp_servers.%s.env]\n", name)
			keys := make([]string, 0, len(srv.Env))
			for k := range srv.Env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "%s = %q\n", k, srv.Env[k])
			}
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
