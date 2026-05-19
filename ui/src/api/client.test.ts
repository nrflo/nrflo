import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// Mock connectionsStore before importing client so requestConfig() is fully controlled
const mockActive = vi.fn()
vi.mock('@/stores/connectionsStore', () => ({
  useConnectionsStore: { getState: () => ({ active: mockActive }) },
}))

import { apiFetch, set401Handler, UnauthenticatedError, ForbiddenError, ApiError } from './client'

const LOCAL_CONN = { id: 'local', name: 'Local', baseURL: '', isLocal: true, activeProject: 'default' }
const REMOTE_CONN = {
  id: 'remote-1',
  name: 'Remote',
  baseURL: 'https://api.example.com',
  isLocal: false,
  token: 'tok-secret',
  activeProject: 'proj-r',
}

function makeResponse(status: number, body?: object) {
  const str = body !== undefined ? JSON.stringify(body) : ''
  return new Response(str || undefined, {
    status,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
  })
}

describe('apiFetch error handling', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    vi.stubGlobal('fetch', fetchMock)
    fetchMock.mockReset()
    set401Handler(vi.fn())
    mockActive.mockReturnValue(LOCAL_CONN)
    window.location = {
      protocol: 'http:',
      host: 'localhost:5175',
      pathname: '/dashboard',
      search: '',
    } as Location
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('throws UnauthenticatedError on 401', async () => {
    fetchMock.mockResolvedValue(makeResponse(401, { error: 'Unauthorized' }))
    await expect(apiFetch('/api/v1/tickets')).rejects.toThrow(UnauthenticatedError)
  })

  it('UnauthenticatedError carries status 401 and name', async () => {
    fetchMock.mockResolvedValue(makeResponse(401, { error: 'Unauthorized' }))
    const err = await apiFetch('/api/v1/tickets').catch((e) => e)
    expect(err.status).toBe(401)
    expect(err.name).toBe('UnauthenticatedError')
  })

  it('throws ForbiddenError on 403', async () => {
    fetchMock.mockResolvedValue(makeResponse(403, { error: 'Forbidden' }))
    await expect(apiFetch('/api/v1/admin')).rejects.toThrow(ForbiddenError)
  })

  it('ForbiddenError carries status 403 and name', async () => {
    fetchMock.mockResolvedValue(makeResponse(403, { error: 'Forbidden' }))
    const err = await apiFetch('/api/v1/admin').catch((e) => e)
    expect(err.status).toBe(403)
    expect(err.name).toBe('ForbiddenError')
  })

  it('throws ApiError with parsed JSON message on generic error', async () => {
    fetchMock.mockResolvedValue(makeResponse(500, { error: 'Internal server error' }))
    const err = await apiFetch('/api/v1/something').catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.message).toBe('Internal server error')
    expect(err.status).toBe(500)
  })

  it('falls back to default message when error body is not JSON', async () => {
    fetchMock.mockResolvedValue(new Response('not json', { status: 500 }))
    const err = await apiFetch('/api/v1/something').catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.message).toBe('Request failed with status 500')
  })

  it('calls registered 401 handler with current path for non-login endpoints', async () => {
    const handler = vi.fn()
    set401Handler(handler)
    fetchMock.mockResolvedValue(makeResponse(401, { error: 'Unauthorized' }))

    await apiFetch('/api/v1/tickets').catch(() => {})

    expect(handler).toHaveBeenCalledOnce()
    expect(handler).toHaveBeenCalledWith('/dashboard', { isLocal: true, connectionId: 'local' })
  })

  it('includes search in path passed to 401 handler', async () => {
    window.location = { ...window.location, pathname: '/tickets', search: '?status=open' } as Location
    const handler = vi.fn()
    set401Handler(handler)
    fetchMock.mockResolvedValue(makeResponse(401, { error: 'Unauthorized' }))

    await apiFetch('/api/v1/tickets').catch(() => {})

    expect(handler).toHaveBeenCalledWith('/tickets?status=open', { isLocal: true, connectionId: 'local' })
  })

  it('does NOT call 401 handler for /api/v1/auth/login endpoint', async () => {
    const handler = vi.fn()
    set401Handler(handler)
    fetchMock.mockResolvedValue(makeResponse(401, { error: 'Wrong password' }))

    await apiFetch('/api/v1/auth/login', { method: 'POST' }).catch(() => {})

    expect(handler).not.toHaveBeenCalled()
  })

  it('returns undefined for 204 No Content', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }))
    const result = await apiFetch('/api/v1/auth/logout')
    expect(result).toBeUndefined()
  })

  it('returns undefined for 205 Reset Content', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 205 }))
    const result = await apiFetch('/api/v1/something')
    expect(result).toBeUndefined()
  })

  it('local connection: sends credentials: include', async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }))
    await apiFetch('/api/v1/something')
    const [, options] = fetchMock.mock.calls[0]
    expect(options.credentials).toBe('include')
  })

  it('local connection: does not send Authorization header', async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({}), { status: 200 }))
    await apiFetch('/api/v1/something')
    const [, options] = fetchMock.mock.calls[0]
    expect((options.headers as Record<string, string>)?.['Authorization']).toBeUndefined()
  })

  it('sends X-Project header', async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({}), { status: 200 }))
    await apiFetch('/api/v1/something')
    const [, options] = fetchMock.mock.calls[0]
    expect((options.headers as Record<string, string>)?.['X-Project']).toBeTruthy()
  })
})

describe('apiFetch — remote connection', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    vi.stubGlobal('fetch', fetchMock)
    fetchMock.mockReset()
    set401Handler(vi.fn())
    mockActive.mockReturnValue(REMOTE_CONN)
    window.location = {
      protocol: 'http:',
      host: 'localhost:5175',
      pathname: '/dashboard',
      search: '',
    } as Location
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('prefixes request URL with remote baseURL', async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({}), { status: 200 }))
    await apiFetch('/api/v1/tickets')
    const [url] = fetchMock.mock.calls[0]
    expect(url).toBe('https://api.example.com/api/v1/tickets')
  })

  it('sends Authorization: Bearer header with remote token', async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({}), { status: 200 }))
    await apiFetch('/api/v1/tickets')
    const [, options] = fetchMock.mock.calls[0]
    expect((options.headers as Record<string, string>)?.['Authorization']).toBe('Bearer tok-secret')
  })

  it('sends credentials: omit for remote connections', async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({}), { status: 200 }))
    await apiFetch('/api/v1/tickets')
    const [, options] = fetchMock.mock.calls[0]
    expect(options.credentials).toBe('omit')
  })

  it('sends X-Project header from remote connection activeProject', async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({}), { status: 200 }))
    await apiFetch('/api/v1/tickets')
    const [, options] = fetchMock.mock.calls[0]
    expect((options.headers as Record<string, string>)?.['X-Project']).toBe('proj-r')
  })

  it('401 handler receives ctx with isLocal=false and remote connectionId', async () => {
    const handler = vi.fn()
    set401Handler(handler)
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ error: 'Unauthorized' }), { status: 401 }))
    await apiFetch('/api/v1/tickets').catch(() => {})
    expect(handler).toHaveBeenCalledWith('/dashboard', { isLocal: false, connectionId: 'remote-1' })
  })
})
