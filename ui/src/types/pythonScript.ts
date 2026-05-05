export interface PythonScript {
  id: string
  project_id: string
  name: string
  description: string
  code: string
  created_at: string
  updated_at: string
}

export interface PythonScriptCreateRequest {
  name: string
  description?: string
  code: string
}

export interface PythonScriptUpdateRequest {
  name?: string
  description?: string
  code?: string
}

export interface ValidationResult {
  ok: boolean
  error?: string
  line?: number
  col?: number
}
