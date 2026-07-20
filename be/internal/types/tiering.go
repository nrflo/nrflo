package types

// TieringDefRow is one agent_definitions row's current-vs-recommended tier
// state within a project's tiering report.
type TieringDefRow struct {
	WorkflowID          string   `json:"workflow_id"`
	DefID               string   `json:"def_id"`
	Role                string   `json:"role"`
	CurrentModel        string   `json:"current_model"`
	CurrentEffort       string   `json:"current_effort,omitempty"`
	RecommendedModel    string   `json:"recommended_model"`
	RecommendedEffort   string   `json:"recommended_effort,omitempty"`
	RecommendedTemplate string   `json:"recommended_template"`
	Customized          bool     `json:"customized"`
	SkipReason          string   `json:"skip_reason,omitempty"`
	EstMonthlyDelta     *float64 `json:"est_monthly_delta,omitempty"`
	IsWorker            bool     `json:"is_worker"`
}

// TieringProjectReport is one project's slice of the tiering report.
type TieringProjectReport struct {
	ProjectID       string          `json:"project_id"`
	ProjectName     string          `json:"project_name"`
	Defs            []TieringDefRow `json:"defs"`
	EstMonthlyDelta *float64        `json:"est_monthly_delta,omitempty"`
}

// TieringReport is the full dry-run response for GET /api/v1/admin/tiering-report.
type TieringReport struct {
	Projects []TieringProjectReport `json:"projects"`
	Markdown string                 `json:"markdown"`
}

// TieringDefKey identifies one agent_definitions row for apply confirmation.
type TieringDefKey struct {
	WorkflowID string `json:"workflow_id"`
	DefID      string `json:"def_id"`
}

// TieringApplyConfirmation is one project's explicit confirmation for
// POST /api/v1/admin/tiering-apply: either an explicit def key list, or
// ConfirmAll to apply every eligible def in the project.
type TieringApplyConfirmation struct {
	ProjectID  string          `json:"project_id"`
	DefKeys    []TieringDefKey `json:"def_keys,omitempty"`
	ConfirmAll bool            `json:"confirm_all,omitempty"`
}

// TieringApplyRequest is the POST /api/v1/admin/tiering-apply request body.
type TieringApplyRequest struct {
	Confirmations []TieringApplyConfirmation `json:"confirmations"`
}

// TieringApplyOutcome is the per-def result of an apply attempt.
type TieringApplyOutcome struct {
	ProjectID  string `json:"project_id"`
	WorkflowID string `json:"workflow_id"`
	DefID      string `json:"def_id"`
	Role       string `json:"role"`
	Outcome    string `json:"outcome"` // applied|unchanged|skipped-customized|skipped-consultant|skipped-hotfix|skipped-non-static|skipped-unconfirmed
	Reason     string `json:"reason,omitempty"`
}

// TieringApplyResult is the POST /api/v1/admin/tiering-apply response.
type TieringApplyResult struct {
	Applied []TieringApplyOutcome `json:"applied"`
	Skipped []TieringApplyOutcome `json:"skipped"`
}
