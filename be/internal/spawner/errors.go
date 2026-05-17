package spawner

import (
	"fmt"
	"strings"
)

// CallbackError signals that an agent completed with CALLBACK status,
// requesting the orchestrator to re-execute from a target layer, agent, or chain.
type CallbackError struct {
	Level        int      // target layer number (mode=layer)
	Instructions string   // callback instructions from agent findings
	AgentType    string   // the agent that triggered the callback
	Mode         string   // "layer" | "agent" | "chain"; empty defaults to "layer"
	TargetAgent  string   // target agent ID (mode=agent)
	Chain        []string // target phase list (mode=chain)
}

func (e *CallbackError) Error() string {
	switch e.Mode {
	case "agent":
		return fmt.Sprintf("callback agent=%s", e.TargetAgent)
	case "chain":
		return fmt.Sprintf("callback chain=[%s]", strings.Join(e.Chain, ","))
	default:
		return fmt.Sprintf("callback to layer %d", e.Level)
	}
}
