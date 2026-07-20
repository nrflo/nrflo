package cli

import (
	"context"
	"errors"
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
	consoleResumeFlag  string
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

With no engine/model flags, the server supplies a searchable picker of live
chats and enabled models. Ctrl+D detaches for later resume; Ctrl+X closes the
server-owned conversation.

Examples:
  nrflo_server console
  nrflo_server console --engine codex --model gpt-5.5 --effort high
  nrflo_server console --resume <session-id>
  nrflo_server console --engine api --model sonnet-5 --token <token>`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if consoleResumeFlag != "" && (cmd.Flags().Changed("engine") || cmd.Flags().Changed("model")) {
			return fmt.Errorf("--resume cannot be combined with --engine or --model")
		}
		choose := consoleResumeFlag == "" && !cmd.Flags().Changed("engine") && !cmd.Flags().Changed("model")
		err := runConsole(cmd.Context(), choose)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	},
}

func init() {
	consoleCmd.Flags().StringVar(&consoleEngineFlag, "engine", "claude", "conversation engine: claude, codex, or api")
	consoleCmd.Flags().StringVar(&consoleModelFlag, "model", "", "model registry id (API engine requires one; CLI engines may use their default)")
	consoleCmd.Flags().StringVar(&consoleProjectFlag, "project", "", "project id (default: NRFLO_PROJECT, cwd match, then global)")
	consoleCmd.Flags().StringVar(&consoleServerFlag, "server", "", "nrflo_server base URL (default: NRFLO_SERVER_URL, then "+defaultConsoleServer+")")
	consoleCmd.Flags().StringVar(&consoleTokenFlag, "token", "", "remote service token (default: NRFLO_MCP_TOKEN)")
	consoleCmd.Flags().StringVar(&consoleResumeFlag, "resume", "", "resume a live console chat by session id")
}

func runConsole(ctx context.Context, choose bool) error {
	server := firstNonempty(consoleServerFlag, os.Getenv("NRFLO_SERVER_URL"), defaultConsoleServer)
	server = strings.TrimRight(server, "/")
	projectHint := firstNonempty(consoleProjectFlag, os.Getenv("NRFLO_PROJECT"))
	token := firstNonempty(consoleTokenFlag, os.Getenv("NRFLO_MCP_TOKEN"))
	useSocket := token == "" && isLocalServer(server)
	if token == "" && !useSocket {
		return fmt.Errorf("service token required for remote server %s: pass --token or set NRFLO_MCP_TOKEN", server)
	}

	selection := consoleui.Selection{ResumeID: consoleResumeFlag, Engine: consoleEngineFlag, Model: consoleModelFlag}
	var sessionID, projectID, bearer string
	if choose {
		var catalog consoleui.Catalog
		var err error
		if useSocket {
			catalog, err = consoleCatalogOverSocket(projectHint)
		} else {
			resolver := newConsoleProjectResolver(server, token, projectHint)
			resolveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			projectID = resolver.resolveSessionProject(resolveCtx)
			cancel()
			client := consoleui.NewClient(consoleui.Config{BaseURL: server, Token: token, Project: projectID})
			catalog, err = client.Catalog(ctx)
		}
		if err != nil {
			return fmt.Errorf("discover console options: %w", err)
		}
		projectID = catalog.ProjectID
		selection, err = consoleui.Select(ctx, catalog)
		if err != nil {
			return err
		}
	}
	if useSocket {
		var mint consoleChatMint
		var err error
		socketProject := firstNonempty(projectID, projectHint)
		if selection.ResumeID != "" {
			mint, err = attachConsoleChatOverSocket(socketProject, selection.ResumeID)
		} else {
			mint, err = mintConsoleChatOverSocket(socketProject, selection.Engine, selection.Model, selection.Effort)
		}
		if err != nil {
			return err
		}
		sessionID, projectID, bearer = mint.SessionID, mint.ProjectID, mint.Token
	} else {
		resolver := newConsoleProjectResolver(server, token, projectHint)
		resolveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		projectID = resolver.resolveSessionProject(resolveCtx)
		cancel()
		if selection.ResumeID != "" {
			sessionID = selection.ResumeID
		} else {
			client := consoleui.NewClient(consoleui.Config{BaseURL: server, Token: token, Project: projectID})
			createCtx, createCancel := context.WithTimeout(ctx, 30*time.Second)
			var err error
			sessionID, err = client.Create(createCtx, selection.Engine, selection.Model, selection.Effort, "")
			createCancel()
			if err != nil {
				return fmt.Errorf("start console chat for project %q: %w", projectID, err)
			}
		}
		bearer = token
	}

	cfg := consoleui.Config{BaseURL: server, Token: bearer, Project: projectID, Session: sessionID}
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
