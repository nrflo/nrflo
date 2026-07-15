package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"be/internal/consoleui"

	"github.com/spf13/cobra"
)

const defaultConsoleServer = "http://127.0.0.1:6587"

var (
	consoleEngineFlag  string
	consoleModelFlag   string
	consoleProjectFlag string
	consoleServerFlag  string
	consoleTokenFlag   string
)

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Open the native nrflo AI console",
	Long: `Open a native terminal UI backed by nrflo_server's provider-agnostic
console chat protocol. The server owns the Claude, Codex, or direct-API engine;
the TUI streams output, renders Markdown, submits approvals, and interrupts the
active turn without terminating the conversation.

A local server needs no token: the trusted Unix socket creates the chat and
returns its scoped bearer. A remote server requires a service token via
--token or NRFLO_MCP_TOKEN. Project resolution is --project, NRFLO_PROJECT,
the working directory's registered project, then the global project.

Examples:
  nrflo_server console
  nrflo_server console --engine codex --model codex_gpt55_high
  nrflo_server console --engine api --model sonnet --token <token>`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runConsole(cmd.Context())
	},
}

func init() {
	consoleCmd.Flags().StringVar(&consoleEngineFlag, "engine", "claude", "conversation engine: claude, codex, or api")
	consoleCmd.Flags().StringVar(&consoleModelFlag, "model", "", "model registry id (API engine requires one; CLI engines may use their default)")
	consoleCmd.Flags().StringVar(&consoleProjectFlag, "project", "", "project id (default: NRFLO_PROJECT, cwd match, then global)")
	consoleCmd.Flags().StringVar(&consoleServerFlag, "server", "", "nrflo_server base URL (default: NRFLO_SERVER_URL, then "+defaultConsoleServer+")")
	consoleCmd.Flags().StringVar(&consoleTokenFlag, "token", "", "remote service token (default: NRFLO_MCP_TOKEN)")
}

func runConsole(ctx context.Context) error {
	server := firstNonempty(consoleServerFlag, os.Getenv("NRFLO_SERVER_URL"), defaultConsoleServer)
	server = strings.TrimRight(server, "/")
	projectHint := firstNonempty(consoleProjectFlag, os.Getenv("NRFLO_PROJECT"))
	token := firstNonempty(consoleTokenFlag, os.Getenv("NRFLO_MCP_TOKEN"))
	useSocket := token == "" && isLocalServer(server)
	if token == "" && !useSocket {
		return fmt.Errorf("service token required for remote server %s: pass --token or set NRFLO_MCP_TOKEN", server)
	}

	var sessionID, projectID, bearer string
	if useSocket {
		mint, err := mintConsoleChatOverSocket(projectHint, consoleEngineFlag, consoleModelFlag)
		if err != nil {
			return err
		}
		sessionID, projectID, bearer = mint.SessionID, mint.ProjectID, mint.Token
	} else {
		resolver := newConsoleProjectResolver(server, token, projectHint)
		resolveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		projectID = resolver.resolveSessionProject(resolveCtx)
		cancel()
		client := consoleui.NewClient(consoleui.Config{BaseURL: server, Token: token, Project: projectID})
		createCtx, createCancel := context.WithTimeout(ctx, 30*time.Second)
		var err error
		sessionID, err = client.Create(createCtx, consoleEngineFlag, consoleModelFlag)
		createCancel()
		if err != nil {
			return fmt.Errorf("start console chat for project %q: %w", projectID, err)
		}
		bearer = token
	}

	cfg := consoleui.Config{BaseURL: server, Token: bearer, Project: projectID, Session: sessionID}
	client := consoleui.NewClient(cfg)
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Close(closeCtx)
	}()
	if err := consoleui.Run(ctx, cfg); err != nil {
		return err
	}
	return nil
}

func newConsoleProjectResolver(server, token, project string) *nrfloHTTPClient {
	client := &nrfloHTTPClient{
		base: server, serviceToken: token, defaultProject: project, hc: &http.Client{},
	}
	if project != "" {
		client.cwdResolved = true
		client.cwdProjectID = project
	}
	return client
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
