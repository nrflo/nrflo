package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	agentConsultConsultant string
	agentConsultQuestion   string
	agentConsultJSON       bool
)

var agentConsultCmd = &cobra.Command{
	Use:   "consult",
	Short: "Synchronously consult an api-mode consultant agent and print its answer",
	Long: `Spawn a consultant agent defined in the same workflow, block until it answers,
and print the answer to stdout. The consultant must have execution_mode=api and
be declared in the workflow definition.

Context is read from environment variables set by the spawner:
  NRF_SESSION_ID          — current agent session ID (required)
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID

Pass --question - to read the question from stdin.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		sessionID := GetSessionID()
		if sessionID == "" {
			return fmt.Errorf("NRF_SESSION_ID env var is required (only spawned agents may consult)")
		}

		question := agentConsultQuestion
		if question == "" || question == "-" {
			raw, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			question = strings.TrimSpace(string(raw))
		}

		if agentConsultConsultant == "" {
			return fmt.Errorf("--consultant is required")
		}
		if question == "" {
			return fmt.Errorf("--question is required (or pass via stdin)")
		}

		reqParams := map[string]interface{}{
			"consultant": agentConsultConsultant,
			"question":   question,
		}
		addSpawnerIDs(reqParams)

		var out struct {
			Answer string `json:"answer"`
		}
		if err := GetClient().ExecuteAndUnmarshalWithReadDeadline("agent.consult", reqParams, &out, 12*time.Minute); err != nil {
			return err
		}

		if agentConsultJSON {
			b, err := json.Marshal(map[string]string{"answer": out.Answer})
			if err != nil {
				return fmt.Errorf("failed to marshal output: %w", err)
			}
			fmt.Println(string(b))
		} else {
			fmt.Println(out.Answer)
		}
		return nil
	},
}

func init() {
	agentConsultCmd.Flags().StringVar(&agentConsultConsultant, "consultant", "", "Consultant agent id")
	agentConsultCmd.Flags().StringVar(&agentConsultQuestion, "question", "", `Question text (or "-" to read stdin)`)
	agentConsultCmd.Flags().BoolVar(&agentConsultJSON, "json", false, `Emit {"answer":...} JSON`)
	agentCmd.AddCommand(agentConsultCmd)
}
