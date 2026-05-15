import { apiGet, getProject, ApiError } from './client'

const API_BASE_URL = import.meta.env.VITE_API_URL || ''

export interface EnvVarCatalogEntry {
  name: string
  feature: string
  description: string
  required: boolean
}

export async function getEnvVarCatalog(): Promise<EnvVarCatalogEntry[]> {
  const resp = await apiGet<{ vars: EnvVarCatalogEntry[] }>('/api/v1/import/env-var-catalog')
  return resp.vars ?? []
}

export class NotConfiguredError extends Error {
  missing: string[]
  constructor(missing: string[]) {
    super(`Missing environment variables: ${missing.join(', ')}`)
    this.name = 'NotConfiguredError'
    this.missing = missing
  }
}

export interface StartImportRequest {
  source: 'github_issue' | 'jira' | 'markdown'
  body: string
}

export interface StartImportResponse {
  instance_id: string
  status?: 'ready' | 'running' | 'failed'
}

export interface AttachedRef {
  url: string
  label?: string
}

export interface ImportPreviewResponse {
  instance_id: string
  status: 'pending' | 'ready' | 'failed'
  title: string
  description: string
  instructions: string
  raw_spec?: string
  source?: string
  suggested_workflow?: string
  attached_refs?: AttachedRef[]
}

export interface CommitImportRequest {
  title: string
  description: string
}

export interface CommitImportResponse {
  ticket_id: string
}

export interface GitHubIssueSummary {
  number: number
  title: string
  state: string
  html_url: string
}

export interface JiraIssueSummary {
  key: string
  summary: string
  status: string
  url?: string
}

// Thin fetch wrapper that detects 412 and throws NotConfiguredError
async function apiFetchWith412<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...options,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      'X-Project': getProject(),
      ...((options.headers as Record<string, string>) ?? {}),
    },
  })

  if (response.status === 412) {
    let body: { error?: string; missing?: string[] } = {}
    try {
      body = await response.json()
    } catch {
      /* ignore */
    }
    throw new NotConfiguredError(body.missing ?? [])
  }

  if (!response.ok) {
    let message = `Request failed with status ${response.status}`
    try {
      const error = await response.json()
      message = error.error || message
    } catch {
      /* ignore */
    }
    throw new ApiError(response.status, message)
  }

  if (response.status === 204 || response.status === 205) {
    return undefined as T
  }

  return response.json()
}

export async function startImport(req: StartImportRequest): Promise<StartImportResponse> {
  return apiFetchWith412<StartImportResponse>('/api/v1/import/spec', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export async function getImportPreview(instanceId: string): Promise<ImportPreviewResponse> {
  return apiGet<ImportPreviewResponse>(`/api/v1/import/spec/${encodeURIComponent(instanceId)}`)
}

export async function commitImport(
  instanceId: string,
  req: CommitImportRequest
): Promise<CommitImportResponse> {
  return apiFetchWith412<CommitImportResponse>(
    `/api/v1/import/spec/${encodeURIComponent(instanceId)}/commit`,
    { method: 'POST', body: JSON.stringify(req) }
  )
}

export async function searchGitHubIssues(
  q: string,
  repo?: string
): Promise<GitHubIssueSummary[]> {
  const params = new URLSearchParams({ q })
  if (repo) params.set('repo', repo)
  const resp = await apiFetchWith412<{ results: GitHubIssueSummary[] | null }>(
    `/api/v1/import/github/search?${params}`
  )
  return resp.results ?? []
}

export async function searchJiraIssues(q: string): Promise<JiraIssueSummary[]> {
  const params = new URLSearchParams({ q })
  const resp = await apiFetchWith412<{ results: JiraIssueSummary[] | null }>(
    `/api/v1/import/jira/search?${params}`
  )
  return resp.results ?? []
}
