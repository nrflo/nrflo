package spawner

import "fmt"

// CallbackError signals that an agent completed with CALLBACK status,
// requesting the orchestrator to re-execute from a target layer.
type CallbackError struct {
	Level        int    // target layer number (the layer field value, not array index)
	Instructions string // callback instructions from agent findings
	AgentType    string // the agent that triggered the callback
}

func (e *CallbackError) Error() string {
	return fmt.Sprintf("callback to layer %d", e.Level)
}
