package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"nrworkflow/internal/client"
	"nrworkflow/internal/service"
	"nrworkflow/internal/types"
)

// AgentConfig holds agent-specific configuration
type AgentConfig struct {
	Model    string `json:"model"`
	MaxTurns int    `json:"max_turns"`
	Timeout  int    `json:"timeout"`
}

// WorkflowConfig represents the workflow configuration
type WorkflowConfig struct {
	CLI struct {
		Default string `json:"default"`
	} `json:"cli"`
	Agents    map[string]AgentConfig `json:"agents"`
	Workflows map[string]WorkflowDef `json:"workflows"`
}

// WorkflowDef represents a workflow definition
type WorkflowDef struct {
	Description string     `json:"description"`
	Categories  []string   `json:"categories"`
	Phases      []PhaseDef `json:"phases"`
}

// PhaseDef represents a phase definition
type PhaseDef struct {
	ID       string   `json:"id"`
	Agent    string   `json:"agent"`
	SkipFor  []string `json:"skip_for,omitempty"`
	Parallel *struct {
		Enabled bool     `json:"enabled"`
		Models  []string `json:"models"`
	} `json:"parallel,omitempty"`
}

// WorkflowState represents the state of a workflow (v4 format)
type WorkflowState struct {
	Version       int                    `json:"version"`
	InitializedAt string                 `json:"initialized_at"`
	CurrentPhase  string                 `json:"current_phase"`
	Category      string                 `json:"category,omitempty"`
	RetryCount    int                    `json:"retry_count"`
	Phases        map[string]PhaseState  `json:"phases"`
	ActiveAgents  map[string]interface{} `json:"active_agents"`
	AgentHistory  []interface{}          `json:"agent_history"`
	AgentRetries  map[string]int         `json:"agent_retries"`
	Findings      map[string]interface{} `json:"findings"`
	ParentSession string                 `json:"parent_session,omitempty"`
}

// PhaseState represents the state of a phase
type PhaseState struct {
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
}

// Add workflow-related commands to root
func init() {
	rootCmd.AddCommand(workflowsCmd)
	rootCmd.AddCommand(workflowInitCmd)
	rootCmd.AddCommand(workflowStatusCmd)
	rootCmd.AddCommand(workflowProgressCmd)
	rootCmd.AddCommand(workflowGetCmd)
	rootCmd.AddCommand(workflowSetCmd)
	rootCmd.AddCommand(phaseCmd)
}

// workflowsCmd lists available workflows
var workflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "List available workflows",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := CheckServer(); err != nil {
			return err
		}

		c := GetClient()

		var workflows map[string]service.WorkflowDef
		if err := c.ExecuteAndUnmarshal("workflow.list", nil, &workflows); err != nil {
			return fmt.Errorf("failed to list workflows: %w", err)
		}

		fmt.Println("Available workflows:")
		fmt.Println()

		for name, wf := range workflows {
			fmt.Printf("  %s\n", name)
			if wf.Description != "" {
				fmt.Printf("    %s\n", wf.Description)
			}
			phases := []string{}
			for _, p := range wf.Phases {
				phases = append(phases, p.ID)
			}
			fmt.Printf("    Phases: %s\n", strings.Join(phases, " -> "))
			fmt.Println()
		}

		return nil
	},
}

var workflowInitWorkflow string

// workflowInitCmd initializes a workflow on a ticket
var workflowInitCmd = &cobra.Command{
	Use:   "init <ticket>",
	Short: "Initialize a workflow on a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		workflowName := workflowInitWorkflow
		if workflowName == "" {
			workflowName = "feature"
		}

		c := GetClient()
		params := map[string]interface{}{
			"ticket_id": ticketID,
			"workflow":  workflowName,
		}

		if err := c.ExecuteAndUnmarshal("workflow.init", params, nil); err != nil {
			return fmt.Errorf("failed to initialize workflow: %w", err)
		}

		fmt.Printf("Initialized %s with workflow '%s'\n", ticketID, workflowName)
		return nil
	},
}

var workflowStatusWorkflow string

// workflowStatusCmd shows human-readable workflow status
var workflowStatusCmd = &cobra.Command{
	Use:   "status <ticket>",
	Short: "Show workflow status for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		c := GetClient()

		params := map[string]interface{}{
			"ticket_id": ticketID,
			"workflow":  workflowStatusWorkflow,
		}

		var state map[string]interface{}
		if err := c.ExecuteAndUnmarshal("workflow.status", params, &state); err != nil {
			return err
		}

		workflowName := state["workflow"].(string)
		currentPhase := ""
		if cp, ok := state["current_phase"].(string); ok {
			currentPhase = cp
		}
		category := ""
		if cat, ok := state["category"].(string); ok {
			category = cat
		}
		retryCount := 0
		if rc, ok := state["retry_count"].(float64); ok {
			retryCount = int(rc)
		}

		fmt.Printf("Ticket: %s\n", ticketID)
		fmt.Printf("Workflow: %s\n", workflowName)
		fmt.Printf("Category: %s\n", nvl(category, "TBD"))
		fmt.Printf("Current Phase: %s\n", currentPhase)
		fmt.Printf("Retry Count: %d\n", retryCount)
		fmt.Println()
		fmt.Println("Phases:")

		phases, _ := state["phases"].(map[string]interface{})
		for phaseID, phaseRaw := range phases {
			ps, _ := phaseRaw.(map[string]interface{})
			status, _ := ps["status"].(string)
			result, _ := ps["result"].(string)

			var mark string
			var suffix string
			switch status {
			case "completed":
				if result == "pass" {
					mark = "✓"
				} else {
					mark = "✗"
				}
				suffix = " - " + result
			case "in_progress":
				mark = ">"
				suffix = " - in_progress"
			case "skipped":
				mark = "-"
				suffix = " - skipped"
			default:
				mark = " "
			}
			fmt.Printf("  [%s] %s%s\n", mark, phaseID, suffix)
		}

		return nil
	},
}

var (
	workflowProgressWorkflow string
	workflowProgressJSON     bool
)

// workflowProgressCmd shows live progress
var workflowProgressCmd = &cobra.Command{
	Use:   "progress <ticket>",
	Short: "Show live progress with agent tracking",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		c := GetClient()

		params := map[string]interface{}{
			"ticket_id": ticketID,
			"workflow":  workflowProgressWorkflow,
		}

		var state map[string]interface{}
		if err := c.ExecuteAndUnmarshal("workflow.status", params, &state); err != nil {
			return err
		}

		if workflowProgressJSON {
			output := map[string]interface{}{
				"ticket_id": ticketID,
				"workflow":  state["workflow"],
				"state":     state,
			}
			outJSON, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(outJSON))
			return nil
		}

		workflowName := state["workflow"].(string)
		currentPhase := ""
		if cp, ok := state["current_phase"].(string); ok {
			currentPhase = cp
		}

		fmt.Printf("Ticket: %s\n", ticketID)
		fmt.Printf("Workflow: %s\n", workflowName)
		fmt.Printf("Current Phase: %s\n", currentPhase)

		activeAgents, _ := state["active_agents"].(map[string]interface{})
		if len(activeAgents) > 0 {
			fmt.Println()
			fmt.Println("Active Agents:")
			for key, agentRaw := range activeAgents {
				agent, _ := agentRaw.(map[string]interface{})
				agentType, _ := agent["agent_type"].(string)
				startedAt, _ := agent["started_at"].(string)
				result, _ := agent["result"].(string)

				status := "running"
				if result != "" {
					status = result
				}

				fmt.Printf("  %s: %s - %s (%s)\n", key, agentType, status, startedAt)
			}
		}

		return nil
	},
}

var workflowGetWorkflow string

// workflowGetCmd gets workflow state
var workflowGetCmd = &cobra.Command{
	Use:   "get <ticket> [field]",
	Short: "Get workflow state or specific field",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		var field string
		if len(args) > 1 {
			field = args[1]
		}

		c := GetClient()
		params := map[string]interface{}{
			"ticket_id": ticketID,
			"workflow":  workflowGetWorkflow,
			"field":     field,
		}

		var result map[string]interface{}
		if err := c.ExecuteAndUnmarshal("workflow.get", params, &result); err != nil {
			return err
		}

		if field != "" {
			fmt.Println(client.FormatValue(result["value"]))
		} else {
			output, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(output))
		}

		return nil
	},
}

var workflowSetWorkflow string

// workflowSetCmd sets a workflow field
var workflowSetCmd = &cobra.Command{
	Use:   "set <ticket> <key> <value>",
	Short: "Set a workflow field",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		key := args[1]
		value := args[2]

		if workflowSetWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required for set command")
		}

		c := GetClient()
		params := types.WorkflowSetRequest{
			Workflow: workflowSetWorkflow,
			Key:      key,
			Value:    value,
		}
		reqParams := map[string]interface{}{
			"ticket_id": ticketID,
			"workflow":  params.Workflow,
			"key":       params.Key,
			"value":     params.Value,
		}

		if err := c.ExecuteAndUnmarshal("workflow.set", reqParams, nil); err != nil {
			return fmt.Errorf("failed to update: %w", err)
		}

		fmt.Printf("Set %s = %s\n", key, value)
		return nil
	},
}

// Phase commands
var phaseCmd = &cobra.Command{
	Use:   "phase",
	Short: "Manage workflow phases",
}

var phaseStartWorkflow string

var phaseStartCmd = &cobra.Command{
	Use:   "start <ticket> <phase>",
	Short: "Start a phase",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		phase := args[1]

		if phaseStartWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		params := map[string]interface{}{
			"ticket_id": ticketID,
			"workflow":  phaseStartWorkflow,
			"phase":     phase,
		}

		if err := c.ExecuteAndUnmarshal("phase.start", params, nil); err != nil {
			return err
		}

		fmt.Printf("Started phase: %s\n", phase)
		return nil
	},
}

var phaseCompleteWorkflow string

var phaseCompleteCmd = &cobra.Command{
	Use:   "complete <ticket> <phase> <result>",
	Short: "Complete a phase (result: pass, fail, skipped)",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		phase := args[1]
		result := args[2]

		if phaseCompleteWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		if result != "pass" && result != "fail" && result != "skipped" {
			return fmt.Errorf("result must be 'pass', 'fail', or 'skipped'")
		}

		c := GetClient()
		params := map[string]interface{}{
			"ticket_id": ticketID,
			"workflow":  phaseCompleteWorkflow,
			"phase":     phase,
			"result":    result,
		}

		if err := c.ExecuteAndUnmarshal("phase.complete", params, nil); err != nil {
			return err
		}

		fmt.Printf("Completed phase: %s (%s)\n", phase, result)
		return nil
	},
}

var phaseReadyWorkflow string

var phaseReadyCmd = &cobra.Command{
	Use:   "ready <ticket> <phase>",
	Short: "Check if phase is ready (all parallel agents done)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		// phase := args[1] // Not used for now

		if phaseReadyWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		params := map[string]interface{}{
			"ticket_id": ticketID,
			"workflow":  phaseReadyWorkflow,
		}

		var state map[string]interface{}
		if err := c.ExecuteAndUnmarshal("workflow.status", params, &state); err != nil {
			fmt.Println("ready")
			return nil
		}

		activeAgents, _ := state["active_agents"].(map[string]interface{})
		if len(activeAgents) == 0 {
			fmt.Println("ready")
			return nil
		}

		// Check if any agent is still running (no result)
		for _, agentRaw := range activeAgents {
			agent, _ := agentRaw.(map[string]interface{})
			if agent["result"] == nil || agent["result"] == "" {
				fmt.Println("pending")
				return nil
			}
		}

		fmt.Println("ready")
		return nil
	},
}

func init() {
	workflowInitCmd.Flags().StringVarP(&workflowInitWorkflow, "workflow", "w", "", "Workflow name")
	workflowStatusCmd.Flags().StringVarP(&workflowStatusWorkflow, "workflow", "w", "", "Workflow name")
	workflowProgressCmd.Flags().StringVarP(&workflowProgressWorkflow, "workflow", "w", "", "Workflow name")
	workflowProgressCmd.Flags().BoolVar(&workflowProgressJSON, "json", false, "Output as JSON")
	workflowGetCmd.Flags().StringVarP(&workflowGetWorkflow, "workflow", "w", "", "Workflow name")
	workflowSetCmd.Flags().StringVarP(&workflowSetWorkflow, "workflow", "w", "", "Workflow name (required)")

	phaseStartCmd.Flags().StringVarP(&phaseStartWorkflow, "workflow", "w", "", "Workflow name (required)")
	phaseCompleteCmd.Flags().StringVarP(&phaseCompleteWorkflow, "workflow", "w", "", "Workflow name (required)")
	phaseReadyCmd.Flags().StringVarP(&phaseReadyWorkflow, "workflow", "w", "", "Workflow name (required)")

	phaseCmd.AddCommand(phaseStartCmd)
	phaseCmd.AddCommand(phaseCompleteCmd)
	phaseCmd.AddCommand(phaseReadyCmd)
}

// Helper functions

// loadMergedWorkflowConfig loads workflow config using the service layer
// (kept name for backward compatibility with agent.go calls)
func loadMergedWorkflowConfig(projectRoot string) (*WorkflowConfig, error) {
	svcConfig, err := service.LoadWorkflowConfig(projectRoot)
	if err != nil {
		return nil, err
	}

	// Convert to CLI types
	config := &WorkflowConfig{
		Agents:    make(map[string]AgentConfig),
		Workflows: make(map[string]WorkflowDef),
	}
	config.CLI.Default = svcConfig.CLI.Default

	for name, ac := range svcConfig.Agents {
		config.Agents[name] = AgentConfig{
			Model:    ac.Model,
			MaxTurns: ac.MaxTurns,
			Timeout:  ac.Timeout,
		}
	}

	for name, wf := range svcConfig.Workflows {
		phases := make([]PhaseDef, len(wf.Phases))
		for i, p := range wf.Phases {
			pd := PhaseDef{
				ID:      p.ID,
				Agent:   p.Agent,
				SkipFor: p.SkipFor,
			}
			// Copy parallel config if present
			if p.Parallel != nil {
				pd.Parallel = &struct {
					Enabled bool     `json:"enabled"`
					Models  []string `json:"models"`
				}{
					Enabled: p.Parallel.Enabled,
					Models:  p.Parallel.Models,
				}
			}
			phases[i] = pd
		}
		config.Workflows[name] = WorkflowDef{
			Description: wf.Description,
			Categories:  wf.Categories,
			Phases:      phases,
		}
	}

	return config, nil
}

// GetProjectRootPath returns the root path for the current project
func GetProjectRootPath() string {
	if ProjectRoot != "" {
		return ProjectRoot
	}
	return "."
}

func nvl(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
