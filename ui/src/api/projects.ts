import { apiGet, apiPost, apiPatch, apiDelete } from './client'

export interface Project {
  id: string
  name: string
  root_path: string | null
  default_branch: string | null
  use_git_worktrees: boolean
  use_docker_isolation: boolean
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
  use_docker_isolation?: boolean
}

export interface UpdateProjectRequest {
  name?: string
  root_path?: string
  default_branch?: string
  use_git_worktrees?: boolean
  use_docker_isolation?: boolean
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
