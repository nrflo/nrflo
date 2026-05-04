export type ReviewStatus = 'pending' | 'approved' | 'rejected'
export type DispatchStatus = 'success' | 'error' | 'pending'
export type NrvappRange = '7d' | '30d'
export type NrvappBucket = '1h' | '6h' | '1d'

export interface NrvappReviewItem {
  id: string
  tool_name: string
  status: ReviewStatus
  input: Record<string, unknown>
  output: Record<string, unknown>
  draft?: Record<string, unknown>
  diff?: string
  created_at: string
  updated_at: string
}

export interface NrvappToolDispatch {
  id: string
  tool_name: string
  status: DispatchStatus
  created_at: string
}

export interface NrvappConfigFileMeta {
  path: string
  latest_version: number
  has_schema: boolean
  updated_at: string
}

export interface NrvappConfigFile {
  path: string
  content: string
  schema?: Record<string, unknown>
  version: number
}

export interface NrvappConfigVersion {
  version: number
  actor: string
  created_at: string
  content?: string
}

export interface NrvappSummary {
  total_dispatches: number
  total_reviews: number
  pending_reviews: number
  approved_rate: number
  reject_rate: number
}

export interface NrvappEditRateRow {
  tool_name: string
  approve_no_edits: number
  approve_with_edits: number
  reject: number
}

export interface NrvappThroughputPoint {
  time: string
  success: number
  error: number
}
