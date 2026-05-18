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

export class UnauthenticatedError extends ApiError {
  constructor(message: string) {
    super(401, message)
    this.name = 'UnauthenticatedError'
  }
}

export class ForbiddenError extends ApiError {
  constructor(message: string) {
    super(403, message)
    this.name = 'ForbiddenError'
  }
}

let currentProject = 'default'
let currentDbPath = ''
let on401: ((path: string) => void) | null = null

export function set401Handler(handler: (path: string) => void) {
  on401 = handler
}

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

  if (dbPathHeader) {
    headers['X-DB-Path'] = dbPathHeader
  }

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...fetchOptions,
    credentials: 'include',
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
    if (response.status === 401) {
      if (endpoint !== '/api/v1/auth/login' && on401) {
        on401(window.location.pathname + window.location.search)
      }
      throw new UnauthenticatedError(message)
    }
    if (response.status === 403) {
      throw new ForbiddenError(message)
    }
    throw new ApiError(response.status, message)
  }

  if (response.status === 204 || response.status === 205) {
    return undefined as T
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

export async function apiPut<T>(
  endpoint: string,
  body?: unknown,
  options?: FetchOptions
): Promise<T> {
  return apiFetch<T>(endpoint, {
    ...options,
    method: 'PUT',
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

export async function apiGetBlob(
  endpoint: string,
  options?: FetchOptions
): Promise<{ blob: Blob; filename: string | null }> {
  const { project, dbPath, ...fetchOptions } = options ?? {}
  const projectHeader = project || currentProject
  const dbPathHeader = dbPath || currentDbPath

  const headers: Record<string, string> = {
    'X-Project': projectHeader,
  }

  if (dbPathHeader) {
    headers['X-DB-Path'] = dbPathHeader
  }

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...fetchOptions,
    method: 'GET',
    credentials: 'include',
    headers: {
      ...headers,
      ...(fetchOptions.headers as Record<string, string> | undefined),
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
    if (response.status === 401) {
      if (endpoint !== '/api/v1/auth/login' && on401) {
        on401(window.location.pathname + window.location.search)
      }
      throw new UnauthenticatedError(message)
    }
    if (response.status === 403) {
      throw new ForbiddenError(message)
    }
    throw new ApiError(response.status, message)
  }

  const blob = await response.blob()
  const cd = response.headers.get('Content-Disposition')
  let filename: string | null = null
  if (cd) {
    const match = cd.match(/filename[^;=\n]*=(['"]?)([^'";\n]+)\1/)
    if (match) filename = match[2]
  }
  return { blob, filename }
}

export async function apiUploadMultipart<T>(
  endpoint: string,
  file: File,
  extraFields?: Record<string, string>
): Promise<T> {
  const formData = new FormData()
  formData.append('file', file)
  if (extraFields) {
    for (const [key, value] of Object.entries(extraFields)) {
      formData.append(key, value)
    }
  }

  const headers: Record<string, string> = {
    'X-Project': currentProject,
  }
  if (currentDbPath) {
    headers['X-DB-Path'] = currentDbPath
  }

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    method: 'POST',
    credentials: 'include',
    headers,
    body: formData,
  })

  if (!response.ok) {
    let message = `Request failed with status ${response.status}`
    try {
      const error = await response.json()
      message = error.error || message
    } catch {
      // ignore parse error
    }
    if (response.status === 401) {
      if (endpoint !== '/api/v1/auth/login' && on401) {
        on401(window.location.pathname + window.location.search)
      }
      throw new UnauthenticatedError(message)
    }
    if (response.status === 403) {
      throw new ForbiddenError(message)
    }
    throw new ApiError(response.status, message)
  }

  if (response.status === 204 || response.status === 205) {
    return undefined as T
  }

  return response.json()
}
