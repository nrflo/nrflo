export interface ServiceToken {
  id: string
  project_id: string
  name: string
  display_hint: string
  created_at: string
  created_by?: string
  last_used_at?: string
}

export interface CreateServiceTokenResponse {
  token: string
  record: ServiceToken
}
