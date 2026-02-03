const API_BASE_URL = import.meta.env.VITE_API_URL || ''

interface FetchOptions extends RequestInit {
  project?: string
  dbPath?: string
}

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

let currentProject = 'default'
let currentDbPath = ''

export function setProject(project: string) {
  currentProject = project
}

export function getProject(): string {
  return currentProject
}

export function setDbPath(path: string) {
  currentDbPath = path
}

export function getDbPath(): string {
  return currentDbPath
}

export async function apiFetch<T>(
  endpoint: string,
  options: FetchOptions = {}
): Promise<T> {
  const { project, dbPath, ...fetchOptions } = options
  const projectHeader = project || currentProject
  const dbPathHeader = dbPath || currentDbPath

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Project': projectHeader,
  }

  // Only add X-DB-Path if we have a path configured
  if (dbPathHeader) {
    headers['X-DB-Path'] = dbPathHeader
  }

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...fetchOptions,
    headers: {
      ...headers,
      ...fetchOptions.headers,
    },
  })

  if (!response.ok) {
    let message = `Request failed with status ${response.status}`
    try {
      const error = await response.json()
      message = error.error || message
    } catch {
      // ignore parse error
    }
    throw new ApiError(response.status, message)
  }

  return response.json()
}

export async function apiGet<T>(
  endpoint: string,
  options?: FetchOptions
): Promise<T> {
  return apiFetch<T>(endpoint, { ...options, method: 'GET' })
}

export async function apiPost<T>(
  endpoint: string,
  body?: unknown,
  options?: FetchOptions
): Promise<T> {
  return apiFetch<T>(endpoint, {
    ...options,
    method: 'POST',
    body: body ? JSON.stringify(body) : undefined,
  })
}

export async function apiPatch<T>(
  endpoint: string,
  body?: unknown,
  options?: FetchOptions
): Promise<T> {
  return apiFetch<T>(endpoint, {
    ...options,
    method: 'PATCH',
    body: body ? JSON.stringify(body) : undefined,
  })
}

export async function apiDelete<T>(
  endpoint: string,
  body?: unknown,
  options?: FetchOptions
): Promise<T> {
  return apiFetch<T>(endpoint, {
    ...options,
    method: 'DELETE',
    body: body ? JSON.stringify(body) : undefined,
  })
}
