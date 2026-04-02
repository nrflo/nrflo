export interface ErrorLog {
  id: string
  project_id: string
  error_type: 'agent' | 'workflow' | 'system'
  instance_id: string
  message: string
  created_at: string
}

export interface ErrorsResponse {
  errors: ErrorLog[]
  total: number
  page: number
  per_page: number
  total_pages: number
}
