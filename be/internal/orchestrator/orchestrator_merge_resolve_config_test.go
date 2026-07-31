package orchestrator

import (
	"context"
	"testing"

	"be/internal/model"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/spawner/apirun/provider"
)

// TestConflictResolverConfig_CarriesLifecycleServices pins the wiring contract
// that regressed to "agent service not configured" in production: the
// resolver's spawner.Config must inherit every tool-backing service from the
// run's baseCfg, not a hand-listed subset. If this assertion is ever
// "simplified" away, a nil AgentSvcReal will silently break agent_finished/
// agent_fail/findings_* for every conflict-resolver session again.
func TestConflictResolverConfig_CarriesLifecycleServices(t *testing.T) {
	env := newTestEnv(t)

	baseCfg := spawner.Config{
		FindingsSvc:        service.NewFindingsService(env.pool, env.orch.clock),
		ProjectFindingsSvc: service.NewProjectFindingsService(env.pool, env.orch.clock),
		AgentSvcReal:       service.NewAgentService(env.pool, env.orch.clock),
		WorkflowSvc:        service.NewWorkflowService(env.pool, env.orch.clock),
		TicketSvc:          service.NewTicketService(env.pool, env.orch.clock),
		ArtifactSvc:        service.NewArtifactService(env.pool, env.orch.clock, env.hub, ""),
		BuildAPIProvider:   dummyBuildAPIProvider,
	}

	wt := &worktreeInfo{
		projectRoot:   "/tmp/original-project-root",
		worktreePath:  "/tmp/wt",
		branchName:    "feature-x",
		defaultBranch: "main",
	}
	sysDef := &model.SystemAgentDefinition{ID: "conflict-resolver", Timeout: 30}
	chain := []service.AgentChainEntry{{ModelID: "sonnet-5"}}

	registerCalled := false
	unregisterCalled := false
	onRegister := func(string, *spawner.Spawner) { registerCalled = true }
	onUnregister := func(string) { unregisterCalled = true }

	cfg := conflictResolverConfig(baseCfg, wt, "sonnet-5", sysDef, chain, onRegister, onUnregister)

	// Lifecycle services must survive the derivation. A nil here reproduces
	// the "agent service not configured" symptom from missingService("agent").
	if cfg.AgentSvcReal == nil {
		t.Error("conflictResolverConfig: AgentSvcReal is nil — resolver sessions would get \"agent service not configured\"")
	}
	if cfg.FindingsSvc == nil {
		t.Error("conflictResolverConfig: FindingsSvc is nil — findings_add would fail in resolver sessions")
	}
	if cfg.ProjectFindingsSvc == nil {
		t.Error("conflictResolverConfig: ProjectFindingsSvc is nil")
	}
	if cfg.WorkflowSvc == nil {
		t.Error("conflictResolverConfig: WorkflowSvc is nil")
	}
	if cfg.TicketSvc == nil {
		t.Error("conflictResolverConfig: TicketSvc is nil")
	}
	if cfg.ArtifactSvc == nil {
		t.Error("conflictResolverConfig: ArtifactSvc is nil")
	}
	if cfg.BuildAPIProvider == nil {
		t.Error("conflictResolverConfig: BuildAPIProvider is nil")
	}

	if cfg.OnSessionRegister == nil {
		t.Fatal("conflictResolverConfig: OnSessionRegister is nil")
	}
	if cfg.OnSessionUnregister == nil {
		t.Fatal("conflictResolverConfig: OnSessionUnregister is nil")
	}
	cfg.OnSessionRegister("sid", nil)
	cfg.OnSessionUnregister("sid")
	if !registerCalled {
		t.Error("conflictResolverConfig: OnSessionRegister was not wired to the passed-in callback")
	}
	if !unregisterCalled {
		t.Error("conflictResolverConfig: OnSessionUnregister was not wired to the passed-in callback")
	}
}

// TestConflictResolverConfig_Overrides verifies the synthetic single-phase
// workflow/agent, ProjectRoot, and RefinerySidecar overrides, and that
// baseCfg's own maps are left untouched (they're shared with the run's phase
// spawners).
func TestConflictResolverConfig_Overrides(t *testing.T) {
	baseWorkflows := map[string]spawner.WorkflowDef{
		"real-workflow": {Phases: []spawner.PhaseDef{{NodeID: "a", Agent: "a", Layer: 0}}},
	}
	baseAgents := map[string]spawner.AgentConfig{
		"real-agent": {Model: "sonnet-5"},
	}
	baseCfg := spawner.Config{
		Workflows:       baseWorkflows,
		Agents:          baseAgents,
		ProjectRoot:     "/tmp/should-be-overridden",
		RefinerySidecar: fakeRefinerySidecar{},
	}

	wt := &worktreeInfo{projectRoot: "/tmp/actual-project-root", branchName: "b", defaultBranch: "main"}
	sysDef := &model.SystemAgentDefinition{ID: "conflict-resolver", Timeout: 15}
	chain := []service.AgentChainEntry{{ModelID: "sonnet-5"}}

	cfg := conflictResolverConfig(baseCfg, wt, "sonnet-5", sysDef, chain, nil, nil)

	if len(cfg.Workflows) != 1 {
		t.Fatalf("Workflows = %d entries, want 1", len(cfg.Workflows))
	}
	if _, ok := cfg.Workflows["_conflict_resolution"]; !ok {
		t.Error("Workflows missing synthetic _conflict_resolution entry")
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("Agents = %d entries, want 1", len(cfg.Agents))
	}
	if _, ok := cfg.Agents["conflict-resolver"]; !ok {
		t.Error("Agents missing synthetic conflict-resolver entry")
	}

	if cfg.ProjectRoot != wt.projectRoot {
		t.Errorf("ProjectRoot = %q, want %q", cfg.ProjectRoot, wt.projectRoot)
	}
	if cfg.RefinerySidecar != nil {
		t.Error("RefinerySidecar should be explicitly nil for one-off system spawns")
	}

	// baseCfg's own maps must not be mutated by the derivation.
	if len(baseWorkflows) != 1 || len(baseAgents) != 1 {
		t.Error("conflictResolverConfig mutated baseCfg's original Workflows/Agents maps")
	}
	if _, ok := baseWorkflows["_conflict_resolution"]; ok {
		t.Error("baseCfg.Workflows leaked the synthetic conflict-resolution entry")
	}
}

func dummyBuildAPIProvider(_ context.Context, _, _ string) (provider.Provider, error) {
	return nil, nil
}

type fakeRefinerySidecar struct{}

func (fakeRefinerySidecar) StartSession(sessionID, projectID, wfiID, nodeID string) {}
func (fakeRefinerySidecar) StopSession(sessionID string)                            {}
func (fakeRefinerySidecar) FoldNow(sessionID string)                                {}
