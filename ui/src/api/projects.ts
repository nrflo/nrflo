import { apiGet, apiPost, apiPatch, apiDelete } from './client'

export interface Project {
  id: string
  name: string
  root_path: string | null
  default_branch: string | null
  use_git_worktrees: boolean
  push_after_merge: boolean
  claude_safety_hook: string | null
  created_at: string
  updated_at: string
}

export interface ProjectsResponse {
  projects: Project[]
}

export interface CreateProjectRequest {
  id: string
  name: string
  root_path?: string
  default_branch?: string
  use_git_worktrees?: boolean
  claude_safety_hook?: string
}

export interface UpdateProjectRequest {
  name?: string
  root_path?: string
  default_branch?: string
  use_git_worktrees?: boolean
  push_after_merge?: boolean
  claude_safety_hook?: string
}

export async function listProjects(): Promise<ProjectsResponse> {
  return apiGet<ProjectsResponse>('/api/v1/projects')
}

export async function getProject(id: string): Promise<Project> {
  return apiGet<Project>(`/api/v1/projects/${encodeURIComponent(id)}`)
}

export async function createProject(data: CreateProjectRequest): Promise<Project> {
  return apiPost<Project>('/api/v1/projects', data)
}

export async function updateProject(id: string, data: UpdateProjectRequest): Promise<Project> {
  return apiPatch<Project>(`/api/v1/projects/${encodeURIComponent(id)}`, data)
}

export async function deleteProject(id: string): Promise<{ message: string }> {
  return apiDelete<{ message: string }>(`/api/v1/projects/${encodeURIComponent(id)}`)
}

export interface SafetyHookCheckRequest {
  config: { enabled: boolean; allow_git: boolean; rm_rf_allowed_paths: string[]; dangerous_patterns: string[] }
  command: string
}

export interface SafetyHookCheckResponse {
  allowed: boolean
  reason: string
}

export async function checkSafetyHook(data: SafetyHookCheckRequest): Promise<SafetyHookCheckResponse> {
  return apiPost<SafetyHookCheckResponse>('/api/v1/safety-hook/check', data)
}
