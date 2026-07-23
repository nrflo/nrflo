package model

// Finding schema names a stepwise step's required_findings entries may use —
// the fixed set service/stepengine validates evidence against.
const (
	FindingSchemaJSONArrayPathChange = "json_array_path_change"
	FindingSchemaNonemptyText        = "nonempty_text"
	FindingSchemaOrderedLines        = "ordered_lines"
)

// ValidFindingSchema reports whether name is one of the fixed finding-schema
// names a RequiredFinding may declare.
func ValidFindingSchema(name string) bool {
	switch name {
	case FindingSchemaJSONArrayPathChange, FindingSchemaNonemptyText, FindingSchemaOrderedLines:
		return true
	default:
		return false
	}
}

// RequiredFinding is a single evidence requirement a step's completion must
// satisfy: a findings key plus which fixed schema its value is checked against.
type RequiredFinding struct {
	Key    string `json:"key"`
	Schema string `json:"schema"`
}

// PathOverlap is a declarative cross-key gate: the path-bearing values of
// the Left group's required_findings keys must share no path with the
// Right group's — e.g. backend and frontend file-ownership lists claiming
// the same file. Evaluated by stepengine.checkPathOverlap against the
// step's already-loaded findings, never by a schema on a single key.
type PathOverlap struct {
	Left  []string `json:"left"`
	Right []string `json:"right"`
}

// StepDefinition is one step in an agent_definitions.steps stepwise sequence.
// RotationAllowed marks whether the orchestrator may rotate the assigned
// model/session when this step stalls or fails (never forced false on the
// last step here — the rotate decision itself excludes the final step).
type StepDefinition struct {
	StepID           string            `json:"step_id"`
	Title            string            `json:"title"`
	Instruction      string            `json:"instruction"`
	RequiredFindings []RequiredFinding `json:"required_findings,omitempty"`
	Checks           []string          `json:"checks,omitempty"`
	RotationAllowed  bool              `json:"rotation_allowed,omitempty"`
	PathOverlap      *PathOverlap      `json:"path_overlap,omitempty"`
}
