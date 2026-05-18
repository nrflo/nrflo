package types

import "be/internal/model"

// WorkflowBundle is the stable v1.0 export format for workflow definitions.
type WorkflowBundle struct {
	Version       string                `json:"version"`
	ExportedAt    string                `json:"exported_at"`
	Workflows     []WorkflowBundleEntry `json:"workflows"`
	PythonScripts []*model.PythonScript `json:"python_scripts"`
}

// WorkflowBundleEntry holds one workflow and all its nested definitions.
type WorkflowBundleEntry struct {
	Workflow      *model.Workflow              `json:"workflow"`
	Agents        []*model.AgentDefinition     `json:"agents"`
	LayerPolicies map[int]string               `json:"layer_policies"`
	Notifications []*model.NotificationChannel `json:"notifications"`
}

// ImportConflicts lists entity IDs that already exist in the target project.
type ImportConflicts struct {
	WorkflowIDs     []string `json:"workflow_ids"`
	PythonScriptIDs []string `json:"python_script_ids"`
}

// ImportRequest is the payload for POST /api/v1/workflows/import.
type ImportRequest struct {
	Bundle WorkflowBundle `json:"bundle"`
	Action string         `json:"action"` // "overwrite", "rename", or "cancel"
}

// ImportResult reports what was created during an import.
type ImportResult struct {
	WorkflowIDs     []string `json:"workflow_ids"`
	PythonScriptIDs []string `json:"python_script_ids"`
	Skipped         bool     `json:"skipped"`
}
