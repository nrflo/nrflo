export type ReviewStatus = 'pending' | 'approved' | 'rejected'

export interface ReviewItem {
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
