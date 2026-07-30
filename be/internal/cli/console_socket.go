package cli

import (
	"fmt"
	"net/url"
	"os"

	"be/internal/client"
	"be/internal/types"
)

// consoleSessionMint is the socket console.session reply: a freshly minted
// console session plus the server-resolved project and accepted ticket.
type consoleSessionMint struct {
	SessionID string `json:"session_id"`
	Token     string `json:"token"`
	ProjectID string `json:"project_id"`
	TicketID  string `json:"ticket_id"`
}

type consoleChatMint struct {
	SessionID string `json:"session_id"`
	Token     string `json:"token"`
	ProjectID string `json:"project_id"`
	Engine    string `json:"engine"`
	Model     string `json:"model"`
}

// isLocalServer reports whether serverURL points at a loopback server — the
// only case where the Unix socket (and thus token-less local auth) is reachable.
// A parse failure or unknown host is treated as remote (fail closed to the
// token path). "0.0.0.0" is an all-interfaces bind address; as a client target
// it means the local host.
func isLocalServer(serverURL string) bool {
	u, err := url.Parse(serverURL)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "::1", "localhost", "0.0.0.0":
		return true
	default:
		return false
	}
}

// mintConsoleSessionOverSocket asks the local server to mint a console session
// over the trusted Unix socket — no service token. The server resolves the
// project (projectHint → cwd match → global) and validates the git-branch
// ticket hint. Returns ServerNotRunningError when the socket is unreachable, so
// the caller can point the user at `nrflo_server serve` (or --token for remote).
func mintConsoleSessionOverSocket(projectHint, ticketHint string) (consoleSessionMint, error) {
	cwd, _ := os.Getwd() // best-effort; server falls back to the global project
	c := client.New(projectHint)
	if !c.IsServerRunning() {
		return consoleSessionMint{}, client.ServerNotRunningError()
	}
	var res consoleSessionMint
	params := map[string]string{"project": projectHint, "cwd": cwd, "ticket_id": ticketHint}
	if err := c.ExecuteAndUnmarshal("console.session", params, &res); err != nil {
		return consoleSessionMint{}, fmt.Errorf("mint console session over socket: %w", err)
	}
	return res, nil
}

func mintConsoleChatOverSocket(projectHint, engine, model, effort, profile string) (consoleChatMint, error) {
	cwd, _ := os.Getwd()
	c := client.New(projectHint)
	if !c.IsServerRunning() {
		return consoleChatMint{}, client.ServerNotRunningError()
	}
	var res consoleChatMint
	params := map[string]string{"project": projectHint, "cwd": cwd, "engine": engine, "model": model, "reasoning_effort": effort, "profile": profile}
	if err := c.ExecuteAndUnmarshal("console.chat", params, &res); err != nil {
		return consoleChatMint{}, fmt.Errorf("start console chat over socket: %w", err)
	}
	return res, nil
}

func consoleCatalogOverSocket(projectHint string) (types.ConsoleCatalog, error) {
	cwd, _ := os.Getwd()
	c := client.New(projectHint)
	if !c.IsServerRunning() {
		return types.ConsoleCatalog{}, client.ServerNotRunningError()
	}
	var result types.ConsoleCatalog
	if err := c.ExecuteAndUnmarshal("console.catalog", map[string]string{
		"project": projectHint, "cwd": cwd,
	}, &result); err != nil {
		return types.ConsoleCatalog{}, fmt.Errorf("discover console options over socket: %w", err)
	}
	return result, nil
}

// deleteConsoleChatOverSocket closes a live chat over the trusted Unix socket
// — delete==close semantics, mirroring attachConsoleChatOverSocket's shape.
func deleteConsoleChatOverSocket(projectHint, sessionID string) error {
	cwd, _ := os.Getwd()
	c := client.New(projectHint)
	if !c.IsServerRunning() {
		return client.ServerNotRunningError()
	}
	var result struct {
		SessionID string `json:"session_id"`
	}
	if err := c.ExecuteAndUnmarshal("console.close", map[string]string{
		"project": projectHint, "cwd": cwd, "session_id": sessionID,
	}, &result); err != nil {
		return fmt.Errorf("close console chat over socket: %w", err)
	}
	return nil
}

func attachConsoleChatOverSocket(projectHint, sessionID string) (consoleChatMint, error) {
	cwd, _ := os.Getwd()
	c := client.New(projectHint)
	if !c.IsServerRunning() {
		return consoleChatMint{}, client.ServerNotRunningError()
	}
	var result consoleChatMint
	if err := c.ExecuteAndUnmarshal("console.attach", map[string]string{
		"project": projectHint, "cwd": cwd, "session_id": sessionID,
	}, &result); err != nil {
		return consoleChatMint{}, fmt.Errorf("attach console chat over socket: %w", err)
	}
	return result, nil
}
