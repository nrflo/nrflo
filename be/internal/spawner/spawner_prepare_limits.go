package spawner

import (
	"context"
	"encoding/json"
	"time"

	"be/internal/logger"
	"be/internal/model"
)

// resolveSpawnLimits loads per-agent restart threshold, fail-restart cap, stall
// timeouts (falling back to global config), and validation commands from the
// agent definition for the CLI/API spawn path.
func (s *Spawner) resolveSpawnLimits(ctx context.Context, agentDef *model.AgentDefinition, agentType string) (effectiveThreshold, maxFailRestarts int, stallStartTimeout, stallRunningTimeout time.Duration, validationCommands []string) {
	effectiveThreshold = defaultContextThreshold
	if agentDef != nil && agentDef.RestartThreshold != nil {
		effectiveThreshold = *agentDef.RestartThreshold
	}
	if agentDef != nil && agentDef.MaxFailRestarts != nil {
		maxFailRestarts = *agentDef.MaxFailRestarts
	}
	stallStartTimeout = defaultStallStartTimeout
	stallRunningTimeout = defaultStallRunningTimeout
	if agentDef != nil && agentDef.StallStartTimeoutSec != nil {
		if *agentDef.StallStartTimeoutSec == 0 {
			stallStartTimeout = 0
		} else {
			stallStartTimeout = time.Duration(*agentDef.StallStartTimeoutSec) * time.Second
		}
	} else if s.config.GlobalStallStartTimeout != nil {
		if *s.config.GlobalStallStartTimeout == 0 {
			stallStartTimeout = 0
		} else {
			stallStartTimeout = time.Duration(*s.config.GlobalStallStartTimeout) * time.Second
		}
	}
	if agentDef != nil && agentDef.StallRunningTimeoutSec != nil {
		if *agentDef.StallRunningTimeoutSec == 0 {
			stallRunningTimeout = 0
		} else {
			stallRunningTimeout = time.Duration(*agentDef.StallRunningTimeoutSec) * time.Second
		}
	} else if s.config.GlobalStallRunningTimeout != nil {
		if *s.config.GlobalStallRunningTimeout == 0 {
			stallRunningTimeout = 0
		} else {
			stallRunningTimeout = time.Duration(*s.config.GlobalStallRunningTimeout) * time.Second
		}
	}

	if agentDef != nil && agentDef.ValidationCommands != "" {
		if jsonErr := json.Unmarshal([]byte(agentDef.ValidationCommands), &validationCommands); jsonErr != nil {
			logger.Warn(ctx, "failed to parse validation_commands", "agent", agentType, "error", jsonErr)
			validationCommands = nil
		}
	}
	return
}
