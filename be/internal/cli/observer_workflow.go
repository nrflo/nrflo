package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	obsWFProjectID    string
	obsWFWorkflowID   string
	obsWFInstanceID   string
	obsWFTargetSessID string
	obsWFLogsLimit    int
	obsWFLogsOffset   int
	obsWFTicketID     string
	obsWFInstructions string
	obsWFScopeType    string
	obsWFDefJSON      string
)

var workflowGroupCmd = &cobra.Command{
	Use:           "workflow",
	Short:         "Observer workflow commands",
	Hidden:        true,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("NRF_OBSERVER") != "1" {
			return unknownCmdErr("observer", cmd.Root().Name())
		}
		if os.Getenv("NRF_OBSERVER_SCOPE") != "workflow" {
			return unknownCmdErr("workflow", cmd.Root().Name())
		}
		return nil
	},
}

func obsWFBaseParams() map[string]interface{} {
	params := map[string]interface{}{}
	if obsWFProjectID != "" {
		params["project_id"] = obsWFProjectID
	}
	if obsWFWorkflowID != "" {
		params["workflow_id"] = obsWFWorkflowID
	}
	return params
}

var obsWorkflowShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Get workflow definition",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runObserverMethod("observer.workflow.show", obsWFBaseParams())
	},
}

var obsWorkflowRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "List workflow runs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runObserverMethod("observer.workflow.runs", obsWFBaseParams())
	},
}

var obsWorkflowFindingsCmd = &cobra.Command{
	Use:   "findings",
	Short: "Get findings for the attached workflow instance",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := obsWFBaseParams()
		if obsWFInstanceID != "" {
			params["instance_id"] = obsWFInstanceID
		}
		return runObserverMethod("observer.workflow.findings", params)
	},
}

var obsWorkflowLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Get agent logs for the most recent (or specified) session",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := map[string]interface{}{}
		if obsWFTargetSessID != "" {
			params["target_session_id"] = obsWFTargetSessID
		}
		if obsWFLogsLimit > 0 {
			params["limit"] = obsWFLogsLimit
		}
		if obsWFLogsOffset > 0 {
			params["offset"] = obsWFLogsOffset
		}
		return runObserverMethod("observer.workflow.logs", params)
	},
}

var obsWorkflowTriggerCmd = &cobra.Command{
	Use:   "trigger",
	Short: "Start a workflow run (mutate)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := obsWFBaseParams()
		if obsWFTicketID != "" {
			params["ticket_id"] = obsWFTicketID
		}
		if obsWFInstructions != "" {
			params["instructions"] = obsWFInstructions
		}
		if obsWFScopeType != "" {
			params["scope_type"] = obsWFScopeType
		}
		return runObserverMethod("observer.workflow.trigger", params)
	},
}

var obsWorkflowRetryFailedCmd = &cobra.Command{
	Use:   "retry-failed",
	Short: "Retry failed workflow from failed layer (mutate)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := map[string]interface{}{}
		if obsWFTargetSessID != "" {
			params["target_session_id"] = obsWFTargetSessID
		}
		return runObserverMethod("observer.workflow.retry_failed", params)
	},
}

var obsWorkflowDefCmd = &cobra.Command{
	Use:           "def",
	Short:         "Workflow definition subcommands",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var obsWorkflowDefUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update workflow definition (mutate)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := obsWFBaseParams()
		if obsWFDefJSON != "" {
			var body map[string]interface{}
			if err := json.Unmarshal([]byte(obsWFDefJSON), &body); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
			for k, v := range body {
				params[k] = v
			}
		}
		return runObserverMethod("observer.workflow.def.update", params)
	},
}

func init() {
	workflowGroupCmd.PersistentFlags().StringVar(&obsWFProjectID, "project-id", "", "Project ID")
	workflowGroupCmd.PersistentFlags().StringVar(&obsWFWorkflowID, "workflow-id", "", "Workflow ID")

	obsWorkflowFindingsCmd.Flags().StringVar(&obsWFInstanceID, "instance-id", "", "Workflow instance ID")

	obsWorkflowLogsCmd.Flags().StringVar(&obsWFTargetSessID, "target-session-id", "", "Target session ID")
	obsWorkflowLogsCmd.Flags().IntVar(&obsWFLogsLimit, "limit", 0, "Max messages to return")
	obsWorkflowLogsCmd.Flags().IntVar(&obsWFLogsOffset, "offset", 0, "Message offset")

	obsWorkflowTriggerCmd.Flags().StringVar(&obsWFTicketID, "ticket-id", "", "Ticket ID")
	obsWorkflowTriggerCmd.Flags().StringVar(&obsWFInstructions, "instructions", "", "Run instructions")
	obsWorkflowTriggerCmd.Flags().StringVar(&obsWFScopeType, "scope-type", "", "Scope type (ticket|project)")

	obsWorkflowRetryFailedCmd.Flags().StringVar(&obsWFTargetSessID, "target-session-id", "", "Target session ID")

	obsWorkflowDefUpdateCmd.Flags().StringVar(&obsWFDefJSON, "json", "", "JSON body (WorkflowDefUpdateRequest)")

	obsWorkflowDefCmd.AddCommand(obsWorkflowDefUpdateCmd)
	workflowGroupCmd.AddCommand(
		obsWorkflowShowCmd,
		obsWorkflowRunsCmd,
		obsWorkflowFindingsCmd,
		obsWorkflowLogsCmd,
		obsWorkflowTriggerCmd,
		obsWorkflowRetryFailedCmd,
		obsWorkflowDefCmd,
	)
}
