export type InsightsRange = '7d' | '30d'
export type InsightsBucket = '1h' | '6h' | '1d'

export interface InsightsSummary {
  total_dispatches: number
  total_reviews: number
  pending_reviews: number
  approved_rate: number
  reject_rate: number
}

export interface EditRateRow {
  tool_name: string
  approve_no_edits: number
  approve_with_edits: number
  reject: number
}

export interface ThroughputPoint {
  time: string
  success: number
  error: number
}
