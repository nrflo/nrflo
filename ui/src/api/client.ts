import { useConnectionsStore, type Connection } from '@/stores/connectionsStore'

interface FetchOptions extends RequestInit {
  project?: string
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

type Handler401 = (path: string, ctx: { isLocal: boolean; connectionId: string }) => void
let on401: Handler401 | null = null

export function set401Handler(handler: Handler401) {
  on401 = handler
}

export interface RequestConfig {
  baseURL: string
  project: string
  auth: string | undefined
  useCookie: boolean
  connectionId: string
  isLocal: boolean
}

export function requestConfig(): RequestConfig {
  const active = useConnectionsStore.getState().active()
  return {
    baseURL: active.baseURL,
    project: active.activeProject ?? 'default',
    auth: active.isLocal ? undefined : active.token,
    useCookie: active.isLocal,
    connectionId: active.id,
    isLocal: active.isLocal,
  }
}

function handle401(endpoint: string, cfg: RequestConfig) {
  if (endpoint !== '/api/v1/auth/login' && on401) {
    on401(window.location.pathname + window.location.search, {
      isLocal: cfg.isLocal,
      connectionId: cfg.connectionId,
    })
  }
}

export async function apiFetch<T>(
  endpoint: string,
  options: FetchOptions = {}
): Promise<T> {
  const { project: optProject, ...fetchOptions } = options
  const cfg = requestConfig()
  const project = optProject ?? cfg.project

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Project': project,
  }
  if (cfg.auth) {
    headers['Authorization'] = `Bearer ${cfg.auth}`
  }

  const response = await fetch(`${cfg.baseURL}${endpoint}`, {
    ...fetchOptions,
    credentials: cfg.useCookie ? 'include' : 'omit',
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
      handle401(endpoint, cfg)
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
  const { project: optProject, ...fetchOptions } = options ?? {}
  const cfg = requestConfig()
  const project = optProject ?? cfg.project

  const headers: Record<string, string> = {
    'X-Project': project,
  }
  if (cfg.auth) {
    headers['Authorization'] = `Bearer ${cfg.auth}`
  }

  const response = await fetch(`${cfg.baseURL}${endpoint}`, {
    ...fetchOptions,
    method: 'GET',
    credentials: cfg.useCookie ? 'include' : 'omit',
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
      handle401(endpoint, cfg)
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

// testConnection bypasses the 401 handler and targets a specific Connection directly.
export async function testConnection(conn: Connection): Promise<{ ok: boolean; status: number; message?: string }> {
  const project = conn.activeProject ?? 'default'
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Project': project,
  }
  if (!conn.isLocal && conn.token) {
    headers['Authorization'] = `Bearer ${conn.token}`
  }
  try {
    const response = await fetch(`${conn.baseURL}/api/v1/projects`, {
      method: 'GET',
      credentials: conn.isLocal ? 'include' : 'omit',
      headers,
    })
    if (!response.ok) {
      let message = `Request failed with status ${response.status}`
      try {
        const error = await response.json()
        message = error.error || message
      } catch { /* ignore */ }
      return { ok: false, status: response.status, message }
    }
    return { ok: true, status: response.status }
  } catch (e) {
    return { ok: false, status: 0, message: e instanceof Error ? e.message : 'Network error' }
  }
}

export async function apiUploadMultipart<T>(
  endpoint: string,
  file: File,
  extraFields?: Record<string, string>
): Promise<T> {
  const cfg = requestConfig()
  const formData = new FormData()
  formData.append('file', file)
  if (extraFields) {
    for (const [key, value] of Object.entries(extraFields)) {
      formData.append(key, value)
    }
  }

  const headers: Record<string, string> = {
    'X-Project': cfg.project,
  }
  if (cfg.auth) {
    headers['Authorization'] = `Bearer ${cfg.auth}`
  }

  const response = await fetch(`${cfg.baseURL}${endpoint}`, {
    method: 'POST',
    credentials: cfg.useCookie ? 'include' : 'omit',
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
      handle401(endpoint, cfg)
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
