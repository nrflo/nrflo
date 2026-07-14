package orchestrator

import (
	"runtime"
	"strings"
	"testing"
	"time"

	ptyPkg "be/internal/pty"
	"be/internal/service"
	"be/internal/spawner"
)

// ── setupInteractivePreStep (plan mode) ───────────────────────────────────────

func TestSetupInteractivePreStep_PlanMode_CreatesSession(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-SIP-1", "Interactive pre-step plan test")
	wfiID := env.initWorkflow(t, "TKT-SIP-1")
	wi := env.getWorkflowInstance(t, wfiID)

	svcWf := service.SpawnerWorkflowDef{
		Phases: []service.SpawnerPhaseDef{
			{Agent: "analyzer", Layer: 0},
			{Agent: "builder", Layer: 1},
		},
	}
	svcAgents := map[string]service.SpawnerAgentConfig{}
	workflows := map[string]spawner.WorkflowDef{}
	agents := map[string]spawner.AgentConfig{}

	var registeredSessionID, registeredCmd string
	var registeredArgs []string
	env.orch.OnRegisterPtyCommand = func(sid string, l ptyPkg.Launch) {
		registeredSessionID = sid
		registeredCmd = l.Command
		registeredArgs = l.Args
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "TKT-SIP-1",
		WorkflowName: "test",
		PlanMode:     true,
	}

	pre, err := env.orch.setupInteractivePreStep(req, wi, svcWf, svcAgents, workflows, agents, t.TempDir(), nil, nil, "")
	if err != nil {
		t.Fatalf("setupInteractivePreStep() error: %v", err)
	}
	t.Cleanup(func() { pre.spawner.CompleteInteractive(pre.sessionID) })

	// Validate returned pre-step
	if pre.sessionID == "" {
		t.Error("expected non-empty sessionID")
	}
	if pre.waitCh == nil {
		t.Error("expected non-nil waitCh")
	}
	if pre.spawner == nil {
		t.Error("expected non-nil spawner")
	}

	// Validate OnRegisterPtyCommand invocation
	if registeredSessionID != pre.sessionID {
		t.Errorf("registered session ID = %q, want %q", registeredSessionID, pre.sessionID)
	}
	if registeredCmd != "claude" {
		t.Errorf("registered cmd = %q, want 'claude'", registeredCmd)
	}
	argsStr := strings.Join(registeredArgs, " ")
	if !strings.Contains(argsStr, "--permission-mode plan") {
		t.Errorf("args missing --permission-mode plan: %v", registeredArgs)
	}
	if !strings.Contains(argsStr, "--disallowed-tools ExitPlanMode") {
		t.Errorf("args missing --disallowed-tools ExitPlanMode: %v", registeredArgs)
	}
	if !strings.Contains(argsStr, pre.sessionID) {
		t.Errorf("args missing session ID: %v", registeredArgs)
	}
	if !strings.Contains(argsStr, "--model claude-opus-4-8") {
		t.Errorf("args missing mapped model --model claude-opus-4-8 (got nrflo ID instead): %v", registeredArgs)
	}

	var status, agentType string
	var queryErr error
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		queryErr = env.pool.QueryRow(
			`SELECT status, agent_type FROM agent_sessions WHERE id = ?`,
			pre.sessionID,
		).Scan(&status, &agentType)
		if queryErr == nil {
			break
		}
		runtime.Gosched()
	}
	if queryErr != nil {
		t.Fatalf("failed to query session: %v", queryErr)
	}
	if status != "user_interactive" {
		t.Errorf("session status = %q, want 'user_interactive'", status)
	}
	if agentType != "planner" {
		t.Errorf("session agent_type = %q, want 'planner'", agentType)
	}
}

func TestSetupInteractivePreStep_PlanMode_UsesL0AgentModel(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-SIP-L0", "Plan mode reads L0 model")
	wfiID := env.initWorkflow(t, "TKT-SIP-L0")
	wi := env.getWorkflowInstance(t, wfiID)

	svcWf := service.SpawnerWorkflowDef{
		Phases: []service.SpawnerPhaseDef{
			{Agent: "analyzer", Layer: 0},
			{Agent: "builder", Layer: 1},
		},
	}
	svcAgents := map[string]service.SpawnerAgentConfig{
		"analyzer": {Model: "sonnet"},
		"builder":  {Model: "opus_4_7"},
	}

	var registeredArgs []string
	env.orch.OnRegisterPtyCommand = func(_ string, l ptyPkg.Launch) {
		registeredArgs = l.Args
	}

	pre, err := env.orch.setupInteractivePreStep(
		RunRequest{ProjectID: env.project, TicketID: "TKT-SIP-L0", WorkflowName: "test", PlanMode: true},
		wi, svcWf, svcAgents,
		map[string]spawner.WorkflowDef{},
		map[string]spawner.AgentConfig{},
		t.TempDir(),
		nil,
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("setupInteractivePreStep() error: %v", err)
	}
	t.Cleanup(func() { pre.spawner.CompleteInteractive(pre.sessionID) })

	argsStr := strings.Join(registeredArgs, " ")
	// "sonnet" passes through ClaudeAdapter.MapModel unchanged
	if !strings.Contains(argsStr, "--model sonnet") {
		t.Errorf("plan mode should use L0 agent's model (sonnet), got args: %v", registeredArgs)
	}
	if strings.Contains(argsStr, "claude-opus-4-7") {
		t.Errorf("plan mode should not fall back to opus_4_7 when L0 has a model: %v", registeredArgs)
	}
}

func TestSetupInteractivePreStep_PlanMode_DBMappedModelOverrides(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-SIP-MM", "DB mapped model override")
	wfiID := env.initWorkflow(t, "TKT-SIP-MM")
	wi := env.getWorkflowInstance(t, wfiID)

	svcWf := service.SpawnerWorkflowDef{
		Phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}},
	}

	var registeredArgs []string
	env.orch.OnRegisterPtyCommand = func(_ string, l ptyPkg.Launch) {
		registeredArgs = l.Args
	}

	modelConfigs := map[string]spawner.ModelConfig{
		"opus_4_8": {CLIType: "claude", MappedModel: "claude-opus-db-override"},
	}

	pre, err := env.orch.setupInteractivePreStep(
		RunRequest{ProjectID: env.project, TicketID: "TKT-SIP-MM", WorkflowName: "test", PlanMode: true},
		wi, svcWf,
		map[string]service.SpawnerAgentConfig{},
		map[string]spawner.WorkflowDef{},
		map[string]spawner.AgentConfig{},
		t.TempDir(),
		modelConfigs,
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("setupInteractivePreStep() error: %v", err)
	}
	t.Cleanup(func() { pre.spawner.CompleteInteractive(pre.sessionID) })

	argsStr := strings.Join(registeredArgs, " ")
	if !strings.Contains(argsStr, "--model claude-opus-db-override") {
		t.Errorf("DB MappedModel should override hardcoded mapping; got args: %v", registeredArgs)
	}
	if strings.Contains(argsStr, "--model opus_4_8") {
		t.Errorf("raw nrflo ID leaked to --model: %v", registeredArgs)
	}
}

func TestSetupInteractivePreStep_PlanMode_NoRegisterPtyCommand_OK(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-SIP-NR", "No register callback test")
	wfiID := env.initWorkflow(t, "TKT-SIP-NR")
	wi := env.getWorkflowInstance(t, wfiID)

	svcWf := service.SpawnerWorkflowDef{
		Phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}},
	}

	// OnRegisterPtyCommand is nil — should not panic
	pre, err := env.orch.setupInteractivePreStep(
		RunRequest{ProjectID: env.project, TicketID: "TKT-SIP-NR", WorkflowName: "test", PlanMode: true},
		wi,
		svcWf,
		map[string]service.SpawnerAgentConfig{},
		map[string]spawner.WorkflowDef{},
		map[string]spawner.AgentConfig{},
		t.TempDir(),
		nil,
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("setupInteractivePreStep() with nil callback error: %v", err)
	}
	t.Cleanup(func() { pre.spawner.CompleteInteractive(pre.sessionID) })
}

func TestSetupInteractivePreStep_EmptyWorkflowReturnsError(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-SIP-EW", "Empty workflow test")
	wfiID := env.initWorkflow(t, "TKT-SIP-EW")
	wi := env.getWorkflowInstance(t, wfiID)

	// Interactive mode with empty phases should fail
	svcWf := service.SpawnerWorkflowDef{Phases: []service.SpawnerPhaseDef{}}

	_, err := env.orch.setupInteractivePreStep(
		RunRequest{ProjectID: env.project, TicketID: "TKT-SIP-EW", WorkflowName: "test", Interactive: true},
		wi,
		svcWf,
		map[string]service.SpawnerAgentConfig{},
		map[string]spawner.WorkflowDef{},
		map[string]spawner.AgentConfig{},
		t.TempDir(),
		nil,
		nil,
		"",
	)
	if err == nil {
		t.Fatal("expected error for empty workflow phases in interactive mode")
	}
}
