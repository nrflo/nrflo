export interface PythonScript {
  id: string
  project_id: string
  kind: 'agent' | 'tool'
  name: string
  description: string
  code: string
  file_path: string
  tool_description?: string
  input_schema?: string
  timeout_sec?: number
  created_at: string
  updated_at: string
}

export interface PythonScriptCreateRequest {
  kind: 'agent' | 'tool'
  name: string
  description?: string
  code: string
  file_path?: string
}

export interface PythonScriptUpdateRequest {
  name?: string
  description?: string
  code?: string
  file_path?: string
}

export interface PythonToolCreateRequest {
  kind: 'tool'
  name: string
  description?: string
  tool_description: string
  input_schema: string
  timeout_sec: number
  code: string
  file_path?: string
}

export interface PythonToolUpdateRequest {
  name?: string
  description?: string
  tool_description?: string
  input_schema?: string
  timeout_sec?: number
  code?: string
  file_path?: string
}

export interface ValidationResult {
  ok: boolean
  error?: string
  line?: number
  col?: number
}

export interface BrowseEntry {
  name: string
  is_dir: boolean
  is_python: boolean
  size: number
  modified_at: string
}

export interface BrowseResponse {
  path: string
  entries: BrowseEntry[]
}

export interface ReadFileResponse {
  path: string
  content: string
}
