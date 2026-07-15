package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"be/internal/console"
	"be/internal/model"

	"github.com/spf13/cobra"
)

// defaultConsoleServer is the local `nrflo_server serve` address, used when
// neither --server nor NRFLO_SERVER_URL is set.
const defaultConsoleServer = "http://127.0.0.1:6587"

var (
	consoleCLIFlag     string
	consoleModelFlag   string
	consoleProjectFlag string
	consoleServerFlag  string
	consoleTokenFlag   string
)

// consoleCmd launches a native CLI (claude, codex) locally, wired to a running
// nrflo_server over an injected `agent mcp-external` console session. Unlike
// every spawner-managed session, this is a HUMAN session at the user's own
// terminal — the command holds zero `if cli == "claude"` branches (Rule 6);
// all provider divergence lives in the console.ConsoleDriver implementations.
var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Launch a native CLI (claude, codex) locally with nrflo tools attached",
	Long: `Launch your own claude or codex CLI locally, wired to a running nrflo_server
over an injected 'agent mcp-external' console session.

This is a HUMAN session at your own terminal, not a spawner-managed one: no
--dangerously-skip-permissions, no --disallowedTools deny-list, no
safety-hook --settings injection (claude), no
--dangerously-bypass-approvals-and-sandbox (codex). Native delegation and the
CLI's own approval/sandbox prompts stay exactly as they are for you normally.

Requires a service token (Settings → Administration → Service Tokens), via
--token or NRFLO_MCP_TOKEN. The project resolves from --project, else
NRFLO_PROJECT, else the working directory matched against project root paths,
else the hidden global project; the server from --server, else NRFLO_SERVER_URL,
else the local default. The console session is closed automatically when the
CLI exits.

--model takes a cli_models registry id: its mapped_model, reasoning_effort and
fallback_models all apply. An id registered for the other --cli errors out; an
id absent from the registry is passed to the CLI as a raw model name.

Example usage:
  nrflo_server console                          # claude, cwd-detected project
  nrflo_server console --cli codex --model codex_gpt55_high
  nrflo_server console --project myproj --token <service-token>`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		exitCode, err := runConsole(cmd.Context())
		if err != nil {
			return err
		}
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	},
}

func init() {
	consoleCmd.Flags().StringVar(&consoleCLIFlag, "cli", "claude", "CLI to launch: claude or codex")
	consoleCmd.Flags().StringVar(&consoleModelFlag, "model", "", "cli_models registry id (omit to use the CLI's own default)")
	consoleCmd.Flags().StringVar(&consoleProjectFlag, "project", "", "project id (default: NRFLO_PROJECT, then cwd auto-detect, then the global project)")
	consoleCmd.Flags().StringVar(&consoleServerFlag, "server", "", "running nrflo_server base URL (default: NRFLO_SERVER_URL, then "+defaultConsoleServer+")")
	consoleCmd.Flags().StringVar(&consoleTokenFlag, "token", "", "service token (default: NRFLO_MCP_TOKEN)")
}

// runConsole is consoleCmd's body, split out of RunE so its defers (driver
// cleanup, session close) run to completion before RunE calls os.Exit —
// os.Exit skips deferred funcs, so RunE must not call it until this returns.
func runConsole(ctx context.Context) (int, error) {
	token := consoleTokenFlag
	if token == "" {
		token = os.Getenv("NRFLO_MCP_TOKEN")
	}
	if token == "" {
		return -1, fmt.Errorf("service token required: pass --token or set NRFLO_MCP_TOKEN (Settings → Administration → Service Tokens)")
	}

	drv, err := console.GetDriver(consoleCLIFlag)
	if err != nil {
		return -1, err
	}
	if err := drv.Probe(); err != nil {
		return -1, err
	}

	// Flag > env > default, matching the mcp-external bridge this command
	// injects (agent_mcp_external.go) — it honors both vars, so the command in
	// front of it must too, or an exported NRFLO_PROJECT would be silently
	// overwritten by the resolved default on the way into the child.
	server := consoleServerFlag
	if server == "" {
		server = os.Getenv("NRFLO_SERVER_URL")
	}
	if server == "" {
		server = defaultConsoleServer
	}
	projectFlag := consoleProjectFlag
	if projectFlag == "" {
		projectFlag = os.Getenv("NRFLO_PROJECT")
	}

	client := &nrfloHTTPClient{
		base:           strings.TrimRight(server, "/"),
		serviceToken:   token,
		defaultProject: projectFlag,
		hc:             &http.Client{},
	}
	if projectFlag != "" {
		// An explicit project short-circuits cwd auto-detect / the projects
		// listing call entirely (resolveSessionProject checks cwdResolved first).
		client.mu.Lock()
		client.cwdResolved = true
		client.cwdProjectID = projectFlag
		client.mu.Unlock()
	}

	openCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err = client.openConsoleSession(openCtx)
	cancel()
	if err != nil {
		return -1, err
	}
	defer client.closeConsoleSession()

	project := client.sessionProjectID()
	client.mu.Lock()
	sessionID, consoleBearer := client.sessionID, client.consoleToken
	client.mu.Unlock()

	var row *model.CLIModel
	if consoleModelFlag != "" {
		row, err = client.resolveCLIModel(ctx, consoleCLIFlag, consoleModelFlag)
		if err != nil {
			return -1, err
		}
	}
	// A nil row means the id isn't in the registry at all — the driver falls
	// back to its own adapter.MapModel/GetReasoningEffort for it.
	var mappedModel, effort, fallbacks string
	if row != nil {
		mappedModel, effort, fallbacks = row.MappedModel, row.ReasoningEffort, row.FallbackModels
	}

	nrfloPath, err := os.Executable()
	if err != nil {
		return -1, fmt.Errorf("resolve nrflo_server path: %w", err)
	}
	workDir, err := os.Getwd()
	if err != nil {
		return -1, fmt.Errorf("resolve working directory: %w", err)
	}

	spec, cleanup, err := drv.Prepare(console.LaunchInput{
		ServerURL:       client.base,
		ProjectID:       project,
		SessionID:       sessionID,
		ConsoleToken:    consoleBearer,
		ServiceToken:    token,
		RawModel:        consoleModelFlag,
		MappedModel:     mappedModel,
		ReasoningEffort: effort,
		FallbackModels:  fallbacks,
		WorkDir:         workDir,
		NrfloPath:       nrfloPath,
		CurrentTicket:   client.sessionTicket(),
	})
	if err != nil {
		return -1, err
	}
	defer cleanup()

	fmt.Fprintf(os.Stderr, "nrflo console: session %s, project %s, cli %s\n", sessionID, project, consoleCLIFlag)
	if ticket := client.sessionTicket(); ticket != "" {
		fmt.Fprintf(os.Stderr, "nrflo console: current ticket %s (from git branch)\n", ticket)
	}

	cmd := buildConsoleCmd(spec)
	return runConsoleChild(cmd)
}
