export interface APICredential {
  id: string
  provider: string
  project_id?: string
  secret_ref: string
  created_at: string
  updated_at: string
}

export interface APICredentialCreateRequest {
  provider: string
  project_id?: string
  secret_ref: string
}

export interface APICredentialUpdateRequest {
  provider?: string
  project_id?: string
  secret_ref?: string
}
