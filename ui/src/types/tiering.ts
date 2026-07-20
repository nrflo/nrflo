// Mirrors be/internal/types/tiering.go — keep field names/shape in sync with
// the Go json tags (the admin tiering-report/apply contract).

export interface TieringDefRow {
  workflow_id: string
  def_id: string
  role: string
  current_model: string
  current_effort?: string
  recommended_model: string
  recommended_effort?: string
  recommended_template: string
  customized: boolean
  skip_reason?: string
  est_monthly_delta: number | null
  is_worker: boolean
}

export interface TieringProjectReport {
  project_id: string
  project_name: string
  defs: TieringDefRow[]
  est_monthly_delta: number | null
}

export interface TieringReport {
  projects: TieringProjectReport[]
  markdown: string
}

export interface TieringDefKey {
  workflow_id: string
  def_id: string
}

export interface TieringApplyConfirmation {
  project_id: string
  def_keys?: TieringDefKey[]
  confirm_all?: boolean
}

export interface TieringApplyRequest {
  confirmations: TieringApplyConfirmation[]
}

export interface TieringApplyOutcome {
  project_id: string
  workflow_id: string
  def_id: string
  role: string
  outcome: string
  reason?: string
}

export interface TieringApplyResult {
  applied: TieringApplyOutcome[]
  skipped: TieringApplyOutcome[]
}
