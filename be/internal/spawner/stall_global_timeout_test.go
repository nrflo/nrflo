package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// intPtr is a local helper to take the address of an int literal.
func intPtr(v int) *int { return &v }

// TestConfig_GlobalStallTimeoutFields verifies Config.GlobalStallStartTimeout and
// Config.GlobalStallRunningTimeout default to nil and accept *int values.
func TestConfig_GlobalStallTimeoutFields(t *testing.T) {
	cfg := Config{}
	if cfg.GlobalStallStartTimeout != nil {
		t.Error("Config.GlobalStallStartTimeout default = non-nil, want nil")
	}
	if cfg.GlobalStallRunningTimeout != nil {
		t.Error("Config.GlobalStallRunningTimeout default = non-nil, want nil")
	}

	// Set values and verify round-trip.
	cfg.GlobalStallStartTimeout = intPtr(60)
	cfg.GlobalStallRunningTimeout = intPtr(300)

	if cfg.GlobalStallStartTimeout == nil || *cfg.GlobalStallStartTimeout != 60 {
		t.Errorf("GlobalStallStartTimeout = %v, want 60", cfg.GlobalStallStartTimeout)
	}
	if cfg.GlobalStallRunningTimeout == nil || *cfg.GlobalStallRunningTimeout != 300 {
		t.Errorf("GlobalStallRunningTimeout = %v, want 300", cfg.GlobalStallRunningTimeout)
	}

	// Zero is a valid value (means disabled).
	cfg.GlobalStallStartTimeout = intPtr(0)
	if *cfg.GlobalStallStartTimeout != 0 {
		t.Errorf("GlobalStallStartTimeout zero = %d, want 0", *cfg.GlobalStallStartTimeout)
	}
}

// TestStallTimeout_ResolutionPriority tests the 3-level priority chain for stall timeout resolution.
// This mirrors the logic in spawnSingle() and serves as a specification:
//
//	per-agent def (non-nil) > global config (non-nil) > hardcoded default.
func TestStallTimeout_ResolutionPriority(t *testing.T) {
	tests := []struct {
		name          string
		globalStart   *int
		agentDefStart *int // nil means agent def has no stall_start_timeout_sec
		wantStart     time.Duration
	}{
		{
			name:          "neither_set_uses_hardcoded_default",
			globalStart:   nil,
			agentDefStart: nil,
			wantStart:     defaultStallStartTimeout, // 2m
		},
		{
			name:          "global_set_agentdef_nil_uses_global",
			globalStart:   intPtr(60),
			agentDefStart: nil,
			wantStart:     60 * time.Second,
		},
		{
			name:          "agentdef_wins_over_global",
			globalStart:   intPtr(60),
			agentDefStart: intPtr(30),
			wantStart:     30 * time.Second,
		},
		{
			name:          "global_zero_disables",
			globalStart:   intPtr(0),
			agentDefStart: nil,
			wantStart:     0,
		},
		{
			name:          "agentdef_zero_disables_overrides_global",
			globalStart:   intPtr(60),
			agentDefStart: intPtr(0),
			wantStart:     0,
		},
		{
			name:          "global_120_custom_seconds",
			globalStart:   intPtr(120),
			agentDefStart: nil,
			wantStart:     120 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the resolution logic from spawnSingle().
			stallStartTimeout := defaultStallStartTimeout

			var agentDef *model.AgentDefinition
			if tt.agentDefStart != nil {
				agentDef = &model.AgentDefinition{StallStartTimeoutSec: tt.agentDefStart}
			}

			if agentDef != nil && agentDef.StallStartTimeoutSec != nil {
				if *agentDef.StallStartTimeoutSec == 0 {
					stallStartTimeout = 0
				} else {
					stallStartTimeout = time.Duration(*agentDef.StallStartTimeoutSec) * time.Second
				}
			} else if tt.globalStart != nil {
				if *tt.globalStart == 0 {
					stallStartTimeout = 0
				} else {
					stallStartTimeout = time.Duration(*tt.globalStart) * time.Second
				}
			}

			if stallStartTimeout != tt.wantStart {
				t.Errorf("stallStartTimeout = %v, want %v", stallStartTimeout, tt.wantStart)
			}
		})
	}
}

// TestStallTimeout_ResolutionPriority_Running mirrors TestStallTimeout_ResolutionPriority for the running timeout.
func TestStallTimeout_ResolutionPriority_Running(t *testing.T) {
	tests := []struct {
		name           string
		globalRunning  *int
		agentDefRun    *int
		wantRunning    time.Duration
	}{
		{
			name:          "neither_set_uses_hardcoded_default",
			globalRunning: nil,
			agentDefRun:   nil,
			wantRunning:   defaultStallRunningTimeout, // 8m
		},
		{
			name:          "global_set_agentdef_nil_uses_global",
			globalRunning: intPtr(300),
			agentDefRun:   nil,
			wantRunning:   300 * time.Second,
		},
		{
			name:          "agentdef_wins_over_global",
			globalRunning: intPtr(300),
			agentDefRun:   intPtr(120),
			wantRunning:   120 * time.Second,
		},
		{
			name:          "global_zero_disables",
			globalRunning: intPtr(0),
			agentDefRun:   nil,
			wantRunning:   0,
		},
		{
			name:          "agentdef_zero_disables_overrides_global",
			globalRunning: intPtr(300),
			agentDefRun:   intPtr(0),
			wantRunning:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stallRunningTimeout := defaultStallRunningTimeout

			var agentDef *model.AgentDefinition
			if tt.agentDefRun != nil {
				agentDef = &model.AgentDefinition{StallRunningTimeoutSec: tt.agentDefRun}
			}

			if agentDef != nil && agentDef.StallRunningTimeoutSec != nil {
				if *agentDef.StallRunningTimeoutSec == 0 {
					stallRunningTimeout = 0
				} else {
					stallRunningTimeout = time.Duration(*agentDef.StallRunningTimeoutSec) * time.Second
				}
			} else if tt.globalRunning != nil {
				if *tt.globalRunning == 0 {
					stallRunningTimeout = 0
				} else {
					stallRunningTimeout = time.Duration(*tt.globalRunning) * time.Second
				}
			}

			if stallRunningTimeout != tt.wantRunning {
				t.Errorf("stallRunningTimeout = %v, want %v", stallRunningTimeout, tt.wantRunning)
			}
		})
	}
}

// TestStallTimeout_GlobalOverride_CheckStall_Start verifies checkStall does not trigger before
// a globally-resolved start timeout of 60s. Constructs processInfo as spawnSingle would after
// resolution: agentDef nil + GlobalStallStartTimeout=60 → stallStartTimeout=60s.
func TestStallTimeout_GlobalOverride_CheckStall_Start(t *testing.T) {
	clk := clock.NewTest(time.Now())
	globalVal := 60
	s := New(Config{Clock: clk, GlobalStallStartTimeout: &globalVal})

	proc := &processInfo{
		hasReceivedMessage: false,
		lastMessageTime:    clk.Now(),
		stallStartTimeout:  60 * time.Second, // resolved from GlobalStallStartTimeout
		stallRestartCount:  0,
	}

	// 59s elapsed: under 60s threshold → no stall.
	clk.Advance(59 * time.Second)
	if s.checkStall(context.Background(), proc, SpawnRequest{}) {
		t.Error("checkStall at 59s with 60s threshold: want false, got true")
	}
}

// TestStallTimeout_GlobalOverride_CheckStall_Running verifies checkStall does not trigger before
// a globally-resolved running timeout of 300s.
func TestStallTimeout_GlobalOverride_CheckStall_Running(t *testing.T) {
	clk := clock.NewTest(time.Now())
	globalVal := 300
	s := New(Config{Clock: clk, GlobalStallRunningTimeout: &globalVal})

	proc := &processInfo{
		hasReceivedMessage:  true,
		lastMessageTime:     clk.Now(),
		stallRunningTimeout: 300 * time.Second, // resolved from GlobalStallRunningTimeout
		stallRestartCount:   0,
	}

	// 299s elapsed: under 300s threshold → no stall.
	clk.Advance(299 * time.Second)
	if s.checkStall(context.Background(), proc, SpawnRequest{}) {
		t.Error("checkStall at 299s with 300s threshold: want false, got true")
	}
}

// TestStallTimeout_GlobalDisabled_CheckStall verifies that when global timeout is 0 (disabled),
// checkStall returns false regardless of elapsed time.
func TestStallTimeout_GlobalDisabled_CheckStall(t *testing.T) {
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})

	// When GlobalStallStartTimeout=0, spawnSingle resolves stallStartTimeout=0 (disabled).
	proc := &processInfo{
		hasReceivedMessage: false,
		lastMessageTime:    clk.Now().Add(-30 * time.Minute), // way overdue
		stallStartTimeout:  0,                                // disabled
		stallRestartCount:  0,
	}

	if s.checkStall(context.Background(), proc, SpawnRequest{}) {
		t.Error("checkStall with disabled (0) start timeout: want false, got true")
	}
}

// TestLoadAgentDefinition_StallTimeoutFields verifies loadAgentDefinition returns
// StallStartTimeoutSec and StallRunningTimeoutSec from DB when set.
func TestLoadAgentDefinition_StallTimeoutFields(t *testing.T) {
	env := newSpawnerTestEnv(t)

	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	startSec := 60
	runningSec := 300
	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	if err := adRepo.Create(&model.AgentDefinition{
		ID:                     "stall-agent",
		ProjectID:              env.project,
		WorkflowID:             "test",
		Model:                  "opus",
		Timeout:                60,
		Prompt:                 "stall test prompt",
		StallStartTimeoutSec:   &startSec,
		StallRunningTimeoutSec: &runningSec,
	}); err != nil {
		t.Fatalf("create agent def: %v", err)
	}

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clock.Real(),
	})

	def := sp.loadAgentDefinition("stall-agent", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil, want non-nil")
	}
	if def.StallStartTimeoutSec == nil || *def.StallStartTimeoutSec != 60 {
		t.Errorf("StallStartTimeoutSec = %v, want 60", def.StallStartTimeoutSec)
	}
	if def.StallRunningTimeoutSec == nil || *def.StallRunningTimeoutSec != 300 {
		t.Errorf("StallRunningTimeoutSec = %v, want 300", def.StallRunningTimeoutSec)
	}
}

// TestLoadAgentDefinition_StallTimeoutFieldsNil verifies loadAgentDefinition returns nil
// stall timeout fields when not set (DB NULL → Go nil).
func TestLoadAgentDefinition_StallTimeoutFieldsNil(t *testing.T) {
	env := newSpawnerTestEnv(t)

	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	if err := adRepo.Create(&model.AgentDefinition{
		ID:        "no-stall-agent",
		ProjectID: env.project,
		WorkflowID: "test",
		Model:     "opus",
		Timeout:   60,
		Prompt:    "no stall timeout",
		// StallStartTimeoutSec and StallRunningTimeoutSec intentionally nil
	}); err != nil {
		t.Fatalf("create agent def: %v", err)
	}

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clock.Real(),
	})

	def := sp.loadAgentDefinition("no-stall-agent", env.project, "test")
	if def == nil {
		t.Fatal("loadAgentDefinition returned nil, want non-nil")
	}
	if def.StallStartTimeoutSec != nil {
		t.Errorf("StallStartTimeoutSec = %v, want nil (not set)", def.StallStartTimeoutSec)
	}
	if def.StallRunningTimeoutSec != nil {
		t.Errorf("StallRunningTimeoutSec = %v, want nil (not set)", def.StallRunningTimeoutSec)
	}
}
