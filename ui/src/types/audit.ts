export interface AuditEntry {
  id: string
  user_id?: string
  action: string
  resource_type: string
  resource_id: string
  ip: string
  user_agent: string
  metadata: string
  created_at: string
}

export interface AuditListResponse {
  items: AuditEntry[]
  total: number
  page: number
  per_page: number
}
