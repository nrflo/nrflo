export type ToolAuthMethod = 'none' | 'bearer_env' | 'bearer_secret_ref'

export interface ToolDefinition {
  id: string
  name: string
  description: string
  input_schema: string
  endpoint: string
  auth_method: ToolAuthMethod
  auth_ref?: string
  timeout_sec: number
  project_id?: string
  workflow_id?: string
  created_at: string
  updated_at: string
}

export interface ToolDefinitionCreateRequest {
  name: string
  description?: string
  input_schema: string
  endpoint: string
  auth_method?: ToolAuthMethod
  auth_ref?: string
  timeout_sec?: number
  project_id?: string
  workflow_id?: string
}

export interface ToolDefinitionUpdateRequest {
  name?: string
  description?: string
  input_schema?: string
  endpoint?: string
  auth_method?: ToolAuthMethod
  auth_ref?: string
  timeout_sec?: number
  project_id?: string
  workflow_id?: string
}
