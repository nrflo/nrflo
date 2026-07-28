package spawner

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestPrepareScriptSpawn_MaxFailRestarts verifies prepareScriptSpawn populates
// proc.maxFailRestarts from agentDef.MaxFailRestarts (via resolveSpawnLimits) and
// that the production canFailRestart() predicate flips false once failRestartCount
// reaches the resolved limit — for both the FAIL and TIMEOUT branches the monitor
// evaluates it against.
func TestPrepareScriptSpawn_MaxFailRestarts(t *testing.T) {
	t.Parallel()
	env := setupScriptSpawnEnv(t)
	t.Cleanup(env.cleanup)

	agentDef := makeMinimalAgentDef(env.scriptID)
	agentDef.MaxFailRestarts = intPtr(2)

	proc, _, err := env.spawner.prepareScriptSpawn(
		context.Background(),
		SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
		"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
		agentDef,
	)
	if err != nil {
		t.Fatalf("prepareScriptSpawn() error: %v", err)
	}

	if proc.maxFailRestarts != 2 {
		t.Fatalf("proc.maxFailRestarts = %d, want 2", proc.maxFailRestarts)
	}

	for _, finalStatus := range []string{"FAIL", "TIMEOUT"} {
		t.Run(finalStatus, func(t *testing.T) {
			p := &processInfo{
				finalStatus:      finalStatus,
				maxFailRestarts:  proc.maxFailRestarts,
				failRestartCount: 0,
			}
			if !p.canFailRestart() {
				t.Fatal("canFailRestart() = false at failRestartCount=0, want true")
			}
			p.failRestartCount = 1
			if !p.canFailRestart() {
				t.Fatal("canFailRestart() = false at failRestartCount=1, want true")
			}
			p.failRestartCount = 2
			if p.canFailRestart() {
				t.Error("canFailRestart() = true at failRestartCount==maxFailRestarts, want false (terminal)")
			}
		})
	}
}

// TestPrepareScriptSpawn_MaxFailRestartsZeroOrNil verifies a script agent def
// with MaxFailRestarts nil or explicitly 0 resolves to proc.maxFailRestarts==0,
// keeping canFailRestart() false (terminal) for both FAIL and TIMEOUT.
func TestPrepareScriptSpawn_MaxFailRestartsZeroOrNil(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		maxFailRestarts *int
	}{
		{name: "nil", maxFailRestarts: nil},
		{name: "explicit_zero", maxFailRestarts: intPtr(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupScriptSpawnEnv(t)
			t.Cleanup(env.cleanup)

			agentDef := makeMinimalAgentDef(env.scriptID)
			agentDef.MaxFailRestarts = tt.maxFailRestarts

			proc, _, err := env.spawner.prepareScriptSpawn(
				context.Background(),
				SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
				"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
				agentDef,
			)
			if err != nil {
				t.Fatalf("prepareScriptSpawn() error: %v", err)
			}
			if proc.maxFailRestarts != 0 {
				t.Fatalf("proc.maxFailRestarts = %d, want 0", proc.maxFailRestarts)
			}

			for _, finalStatus := range []string{"FAIL", "TIMEOUT"} {
				p := &processInfo{
					finalStatus:      finalStatus,
					maxFailRestarts:  proc.maxFailRestarts,
					failRestartCount: 0,
				}
				if p.canFailRestart() {
					t.Errorf("%s: canFailRestart() = true, want false (terminal, maxFailRestarts=0)", finalStatus)
				}
			}
		})
	}
}

// TestPrepareScriptSpawn_StallTimeouts_Regression pins the script-only
// start-stall override: nil StallStartTimeoutSec keeps stallStartTimeout at 0
// (disabled) even when Config.GlobalStallStartTimeout is set — proving the
// shared resolveSpawnLimits reuse did not import the cli global-fallback
// default for scripts. An explicit StallStartTimeoutSec is still honored, and
// stallRunningTimeout keeps resolving through the shared resolver's default.
func TestPrepareScriptSpawn_StallTimeouts_Regression(t *testing.T) {
	t.Parallel()

	t.Run("nil_stall_fields_with_global_config_stays_disabled", func(t *testing.T) {
		env := setupScriptSpawnEnv(t)
		t.Cleanup(env.cleanup)
		env.spawner.config.GlobalStallStartTimeout = intPtr(60)

		agentDef := makeMinimalAgentDef(env.scriptID)
		proc, _, err := env.spawner.prepareScriptSpawn(
			context.Background(),
			SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
			"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
			agentDef,
		)
		if err != nil {
			t.Fatalf("prepareScriptSpawn() error: %v", err)
		}
		if proc.stallStartTimeout != 0 {
			t.Errorf("stallStartTimeout = %v, want 0 (scripts ignore GlobalStallStartTimeout unless def opts in)", proc.stallStartTimeout)
		}
		if proc.stallRunningTimeout != defaultStallRunningTimeout {
			t.Errorf("stallRunningTimeout = %v, want default %v", proc.stallRunningTimeout, defaultStallRunningTimeout)
		}
	})

	t.Run("explicit_stall_start_timeout_honored", func(t *testing.T) {
		env := setupScriptSpawnEnv(t)
		t.Cleanup(env.cleanup)

		agentDef := makeMinimalAgentDef(env.scriptID)
		agentDef.StallStartTimeoutSec = intPtr(30)
		proc, _, err := env.spawner.prepareScriptSpawn(
			context.Background(),
			SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
			"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
			agentDef,
		)
		if err != nil {
			t.Fatalf("prepareScriptSpawn() error: %v", err)
		}
		if proc.stallStartTimeout != 30*time.Second {
			t.Errorf("stallStartTimeout = %v, want 30s", proc.stallStartTimeout)
		}
	})
}

// TestPrepareScriptSpawn_ValidationCommands verifies validation_commands JSON is
// parsed into proc.validationCommands via the shared resolver, and malformed
// JSON degrades to nil without error — parity with the deleted inline block.
func TestPrepareScriptSpawn_ValidationCommands(t *testing.T) {
	t.Parallel()

	t.Run("valid_json", func(t *testing.T) {
		env := setupScriptSpawnEnv(t)
		t.Cleanup(env.cleanup)

		agentDef := makeMinimalAgentDef(env.scriptID)
		agentDef.ValidationCommands = `["go build ./...", "go vet ./..."]`

		proc, _, err := env.spawner.prepareScriptSpawn(
			context.Background(),
			SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
			"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
			agentDef,
		)
		if err != nil {
			t.Fatalf("prepareScriptSpawn() error: %v", err)
		}
		if len(proc.validationCommands) != 2 || proc.validationCommands[0] != "go build ./..." || proc.validationCommands[1] != "go vet ./..." {
			t.Errorf("validationCommands = %v, want [go build ./... go vet ./...]", proc.validationCommands)
		}
	})

	t.Run("malformed_json_degrades_to_nil", func(t *testing.T) {
		env := setupScriptSpawnEnv(t)
		t.Cleanup(env.cleanup)

		agentDef := makeMinimalAgentDef(env.scriptID)
		agentDef.ValidationCommands = `not-json`

		proc, _, err := env.spawner.prepareScriptSpawn(
			context.Background(),
			SpawnRequest{ProjectID: env.projectID, AgentType: "test-agent"},
			"L0", uuid.New().String(), "agent-1", uuid.New().String(), "tok",
			agentDef,
		)
		if err != nil {
			t.Fatalf("prepareScriptSpawn() error: %v", err)
		}
		if proc.validationCommands != nil {
			t.Errorf("validationCommands = %v, want nil for malformed JSON", proc.validationCommands)
		}
	})
}
