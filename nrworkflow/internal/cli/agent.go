package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"nrworkflow/internal/db"
	"nrworkflow/internal/repo"
	"nrworkflow/internal/service"
	"nrworkflow/internal/spawner"
	"nrworkflow/internal/types"
)

// Add agent command to root
func init() {
	rootCmd.AddCommand(agentCmd)
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agents",
}

// Agent spawn flags
var (
	agentSpawnWorkflow string
	agentSpawnSession  string
	agentSpawnCLI      string
)

// Note: spawn command uses direct DB access and spawner, not socket client
// This is because spawn runs in the foreground and manages the agent process
var agentSpawnCmd = &cobra.Command{
	Use:   "spawn <agent-type> <ticket>",
	Short: "Spawn an agent to work on a ticket",
	Long: `Spawn an agent to work on a ticket.

Requires --session (parent session UUID) and -w (workflow name).
The agent runs in the foreground and updates ticket state automatically.

NOTE: The spawn command does not use the socket server - it runs the agent directly.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}

		agentType := args[0]
		ticketID := args[1]

		if agentSpawnWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}
		if agentSpawnSession == "" {
			return fmt.Errorf("--session is required")
		}

		// Get project root path
		projectRoot := GetProjectRootPath()

		// Load config with project overrides
		config, err := loadMergedWorkflowConfig(projectRoot)
		if err != nil {
			return err
		}

		// Convert workflows for spawner
		workflows := make(map[string]spawner.WorkflowDef)
		for name, wf := range config.Workflows {
			phases := make([]spawner.PhaseDef, len(wf.Phases))
			for i, p := range wf.Phases {
				pd := spawner.PhaseDef{
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
			workflows[name] = spawner.WorkflowDef{
				Description: wf.Description,
				Categories:  wf.Categories,
				Phases:      phases,
			}
		}

		// Convert agents config for spawner
		agents := make(map[string]spawner.AgentConfig)
		for name, ac := range config.Agents {
			agents[name] = spawner.AgentConfig{
				Model:    ac.Model,
				MaxTurns: ac.MaxTurns,
				Timeout:  ac.Timeout,
			}
		}

		// Create spawner
		sp := spawner.New(spawner.Config{
			Workflows:   workflows,
			Agents:      agents,
			DefaultCLI:  config.CLI.Default,
			DataPath:    DataPath,
			ProjectRoot: projectRoot,
		})

		// Handle Ctrl+C gracefully
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Println("\nReceived interrupt, stopping agent...")
		}()

		// Spawn and monitor
		return sp.Spawn(spawner.SpawnRequest{
			AgentType:     agentType,
			TicketID:      ticketID,
			ProjectID:     ProjectID,
			WorkflowName:  agentSpawnWorkflow,
			ParentSession: agentSpawnSession,
			CLIName:       agentSpawnCLI,
		})
	},
}

// Agent preview flags
var agentPreviewWorkflow string

// Note: preview also uses direct access since it needs config loading
var agentPreviewCmd = &cobra.Command{
	Use:   "preview <agent-type> <ticket>",
	Short: "Preview the agent prompt without spawning",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}

		agentType := args[0]
		ticketID := args[1]

		// Get project root path
		projectRoot := GetProjectRootPath()

		// Load config with project overrides
		config, err := loadMergedWorkflowConfig(projectRoot)
		if err != nil {
			return err
		}

		// Convert workflows for spawner
		workflows := make(map[string]spawner.WorkflowDef)
		for name, wf := range config.Workflows {
			phases := make([]spawner.PhaseDef, len(wf.Phases))
			for i, p := range wf.Phases {
				pd := spawner.PhaseDef{
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
			workflows[name] = spawner.WorkflowDef{
				Description: wf.Description,
				Categories:  wf.Categories,
				Phases:      phases,
			}
		}

		// Convert agents config for spawner
		agents := make(map[string]spawner.AgentConfig)
		for name, ac := range config.Agents {
			agents[name] = spawner.AgentConfig{
				Model:    ac.Model,
				MaxTurns: ac.MaxTurns,
				Timeout:  ac.Timeout,
			}
		}

		sp := spawner.New(spawner.Config{
			Workflows:   workflows,
			Agents:      agents,
			DefaultCLI:  config.CLI.Default,
			DataPath:    DataPath,
			ProjectRoot: projectRoot,
		})

		prompt, err := sp.Preview(agentType, ticketID, agentPreviewWorkflow)
		if err != nil {
			return err
		}

		fmt.Println(prompt)
		return nil
	},
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available agent types",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := CheckServer(); err != nil {
			return err
		}

		c := GetClient()

		var agents []string
		if err := c.ExecuteAndUnmarshal("agent.list", nil, &agents); err != nil {
			return fmt.Errorf("failed to list agents: %w", err)
		}

		fmt.Println("Available agent types:")
		for _, agent := range agents {
			fmt.Printf("  %s\n", agent)
		}

		return nil
	},
}

// Agent active flags
var agentActiveWorkflow string

var agentActiveCmd = &cobra.Command{
	Use:   "active <ticket>",
	Short: "List active agents for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]

		if agentActiveWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		params := map[string]interface{}{
			"ticket_id": ticketID,
			"workflow":  agentActiveWorkflow,
		}

		var agents []service.ActiveAgent
		if err := c.ExecuteAndUnmarshal("agent.active", params, &agents); err != nil {
			return err
		}

		output, _ := json.MarshalIndent(agents, "", "  ")
		fmt.Println(string(output))
		return nil
	},
}

// Agent start flags - for manual agent registration (used internally)
var (
	agentStartWorkflow string
	agentStartPID      int
	agentStartSession  string
	agentStartModel    string
)

var agentStartCmd = &cobra.Command{
	Use:   "start <ticket> <agent-id> <agent-type>",
	Short: "Register an agent start",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}

		ticketID := args[0]
		agentID := args[1]
		agentType := args[2]

		if agentStartWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		// Use direct DB access for this internal command
		database, err := db.Open(DataPath)
		if err != nil {
			return err
		}
		defer database.Close()

		return updateAgentState(database, ticketID, agentStartWorkflow, "start", agentID, agentType, "", agentStartPID, agentStartSession, agentStartModel)
	},
}

// Agent stop flags
var (
	agentStopWorkflow string
	agentStopModel    string
)

var agentStopCmd = &cobra.Command{
	Use:   "stop <ticket> <agent-id> <result>",
	Short: "Register an agent stop",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}

		ticketID := args[0]
		agentID := args[1]
		result := args[2]

		if agentStopWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		// Use direct DB access for this internal command
		database, err := db.Open(DataPath)
		if err != nil {
			return err
		}
		defer database.Close()

		return updateAgentState(database, ticketID, agentStopWorkflow, "stop", agentID, "", result, 0, "", agentStopModel)
	},
}

// Agent complete/fail flags
var (
	agentCompleteWorkflow string
	agentCompleteModel    string
	agentFailWorkflow     string
	agentFailModel        string
	agentFailReason       string
)

var agentCompleteCmd = &cobra.Command{
	Use:   "complete <ticket> <agent-type>",
	Short: "Mark an agent as completed successfully",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		agentType := args[1]

		if agentCompleteWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		params := types.AgentCompleteRequest{
			Workflow:  agentCompleteWorkflow,
			AgentType: agentType,
			Model:     agentCompleteModel,
		}
		reqParams := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   params.Workflow,
			"agent_type": params.AgentType,
		}
		if params.Model != "" {
			reqParams["model"] = params.Model
		}

		if err := c.ExecuteAndUnmarshal("agent.complete", reqParams, nil); err != nil {
			return err
		}

		fmt.Printf("Agent %s marked as pass\n", agentType)
		return nil
	},
}

var agentFailCmd = &cobra.Command{
	Use:   "fail <ticket> <agent-type>",
	Short: "Mark an agent as failed",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		agentType := args[1]

		if agentFailWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		params := types.AgentCompleteRequest{
			Workflow:  agentFailWorkflow,
			AgentType: agentType,
			Model:     agentFailModel,
		}
		reqParams := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   params.Workflow,
			"agent_type": params.AgentType,
		}
		if params.Model != "" {
			reqParams["model"] = params.Model
		}

		if err := c.ExecuteAndUnmarshal("agent.fail", reqParams, nil); err != nil {
			return err
		}

		fmt.Printf("Agent %s marked as fail\n", agentType)
		return nil
	},
}

// Agent kill flags
var (
	agentKillWorkflow string
	agentKillModel    string
)

var agentKillCmd = &cobra.Command{
	Use:   "kill <ticket>",
	Short: "Kill active agent(s)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]

		if agentKillWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		params := types.AgentKillRequest{
			Workflow: agentKillWorkflow,
			Model:    agentKillModel,
		}
		reqParams := map[string]interface{}{
			"ticket_id": ticketID,
			"workflow":  params.Workflow,
		}
		if params.Model != "" {
			reqParams["model"] = params.Model
		}

		var result map[string]int
		if err := c.ExecuteAndUnmarshal("agent.kill", reqParams, &result); err != nil {
			return err
		}

		killed := result["killed"]
		if killed == 0 {
			fmt.Println("No matching agents found")
		} else {
			fmt.Printf("Killed %d agent(s)\n", killed)
		}

		return nil
	},
}

// Agent retry flags
var (
	agentRetryWorkflow string
	agentRetryModel    string
)

var agentRetryCmd = &cobra.Command{
	Use:   "retry <ticket>",
	Short: "Retry failed agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}

		ticketID := args[0]

		if agentRetryWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		// Use direct DB access
		database, err := db.Open(DataPath)
		if err != nil {
			return err
		}
		defer database.Close()

		ticketRepo := repo.NewTicketRepo(database)
		ticket, err := ticketRepo.Get(ProjectID, ticketID)
		if err != nil {
			return err
		}

		if !ticket.AgentsState.Valid {
			return fmt.Errorf("ticket not initialized")
		}

		var allState map[string]interface{}
		if err := json.Unmarshal([]byte(ticket.AgentsState.String), &allState); err != nil {
			return err
		}

		stateRaw, ok := allState[agentRetryWorkflow]
		if !ok {
			return fmt.Errorf("workflow '%s' not found", agentRetryWorkflow)
		}

		state, _ := stateRaw.(map[string]interface{})

		// Increment retry count
		retryCount, _ := state["retry_count"].(float64)
		state["retry_count"] = int(retryCount) + 1

		allState[agentRetryWorkflow] = state
		stateJSON, _ := json.Marshal(allState)
		stateStr := string(stateJSON)
		fields := &repo.UpdateFields{AgentsState: &stateStr}
		if err := ticketRepo.Update(ProjectID, ticketID, fields); err != nil {
			return err
		}

		fmt.Printf("Retry count incremented to %d\n", int(retryCount)+1)
		return nil
	},
}

func init() {
	// agent spawn
	agentSpawnCmd.Flags().StringVarP(&agentSpawnWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentSpawnCmd.Flags().StringVar(&agentSpawnSession, "session", "", "Parent session UUID (required)")
	agentSpawnCmd.Flags().StringVar(&agentSpawnCLI, "cli", "", "CLI to use (claude, opencode)")
	agentCmd.AddCommand(agentSpawnCmd)

	// agent preview
	agentPreviewCmd.Flags().StringVarP(&agentPreviewWorkflow, "workflow", "w", "", "Workflow name")
	agentCmd.AddCommand(agentPreviewCmd)

	// agent list
	agentCmd.AddCommand(agentListCmd)

	// agent active
	agentActiveCmd.Flags().StringVarP(&agentActiveWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentCmd.AddCommand(agentActiveCmd)

	// agent start
	agentStartCmd.Flags().StringVarP(&agentStartWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentStartCmd.Flags().IntVar(&agentStartPID, "pid", 0, "Process ID")
	agentStartCmd.Flags().StringVar(&agentStartSession, "session", "", "Session ID")
	agentStartCmd.Flags().StringVar(&agentStartModel, "model", "", "Model ID (cli:model format)")
	agentCmd.AddCommand(agentStartCmd)

	// agent stop
	agentStopCmd.Flags().StringVarP(&agentStopWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentStopCmd.Flags().StringVar(&agentStopModel, "model", "", "Model ID")
	agentCmd.AddCommand(agentStopCmd)

	// agent complete
	agentCompleteCmd.Flags().StringVarP(&agentCompleteWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentCompleteCmd.Flags().StringVar(&agentCompleteModel, "model", "", "Model ID")
	agentCmd.AddCommand(agentCompleteCmd)

	// agent fail
	agentFailCmd.Flags().StringVarP(&agentFailWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentFailCmd.Flags().StringVar(&agentFailModel, "model", "", "Model ID")
	agentFailCmd.Flags().StringVar(&agentFailReason, "reason", "", "Failure reason")
	agentCmd.AddCommand(agentFailCmd)

	// agent kill
	agentKillCmd.Flags().StringVarP(&agentKillWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentKillCmd.Flags().StringVar(&agentKillModel, "model", "", "Kill specific model only")
	agentCmd.AddCommand(agentKillCmd)

	// agent retry
	agentRetryCmd.Flags().StringVarP(&agentRetryWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentRetryCmd.Flags().StringVar(&agentRetryModel, "model", "", "Model ID")
	agentCmd.AddCommand(agentRetryCmd)
}

// Helper functions for direct DB access (used by spawn-related commands)

func updateAgentState(database *db.DB, ticketID, workflowName, action, agentID, agentType, result string, pid int, session, modelID string) error {
	ticketRepo := repo.NewTicketRepo(database)
	ticket, err := ticketRepo.Get(ProjectID, ticketID)
	if err != nil {
		return err
	}

	if !ticket.AgentsState.Valid || ticket.AgentsState.String == "" {
		return fmt.Errorf("ticket %s not initialized", ticketID)
	}

	var allState map[string]interface{}
	if err := json.Unmarshal([]byte(ticket.AgentsState.String), &allState); err != nil {
		return fmt.Errorf("failed to parse state: %w", err)
	}

	stateRaw, ok := allState[workflowName]
	if !ok {
		return fmt.Errorf("workflow '%s' not found", workflowName)
	}

	state, _ := stateRaw.(map[string]interface{})

	switch action {
	case "start":
		activeAgents, _ := state["active_agents"].(map[string]interface{})
		if activeAgents == nil {
			activeAgents = make(map[string]interface{})
		}

		cli, model := parseModelID(modelID)
		effectiveModelID := modelID
		if effectiveModelID == "" {
			effectiveModelID = "default"
		}

		key := agentType + ":" + effectiveModelID
		activeAgents[key] = map[string]interface{}{
			"agent_id":   agentID,
			"agent_type": agentType,
			"model_id":   effectiveModelID,
			"cli":        cli,
			"model":      model,
			"pid":        pid,
			"session_id": session,
			"started_at": time.Now().UTC().Format(time.RFC3339),
			"result":     nil,
		}
		state["active_agents"] = activeAgents

		fmt.Printf("Agent started: %s (%s)\n", agentType, agentID)
		if modelID != "" {
			fmt.Printf("  Model: %s\n", modelID)
		}
		if pid != 0 {
			fmt.Printf("  PID: %d\n", pid)
		}

	case "stop":
		activeAgents, _ := state["active_agents"].(map[string]interface{})
		history, _ := state["agent_history"].([]interface{})
		if history == nil {
			history = []interface{}{}
		}

		// Find and remove agent
		var active map[string]interface{}
		var activeKey string
		for k, v := range activeAgents {
			a, _ := v.(map[string]interface{})
			if a["agent_id"] == agentID {
				active = a
				activeKey = k
				break
			}
		}

		if active != nil {
			historyEntry := map[string]interface{}{
				"agent_id":   agentID,
				"agent_type": active["agent_type"],
				"model_id":   active["model_id"],
				"phase":      state["current_phase"],
				"started_at": active["started_at"],
				"ended_at":   time.Now().UTC().Format(time.RFC3339),
				"result":     result,
			}
			history = append(history, historyEntry)
			state["agent_history"] = history

			delete(activeAgents, activeKey)
			state["active_agents"] = activeAgents

			fmt.Printf("Agent stopped: %s (%s)\n", active["agent_type"], result)
		} else {
			fmt.Printf("Agent stopped: %s (%s)\n", agentID, result)
		}
	}

	allState[workflowName] = state
	stateJSON, _ := json.Marshal(allState)
	stateStr := string(stateJSON)
	fields := &repo.UpdateFields{AgentsState: &stateStr}
	return ticketRepo.Update(ProjectID, ticketID, fields)
}

func parseModelID(modelID string) (cli, model string) {
	if modelID == "" || !strings.Contains(modelID, ":") {
		return "claude", modelID
	}
	parts := strings.SplitN(modelID, ":", 2)
	return parts[0], parts[1]
}

// For backwards compatibility
func ParseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}
