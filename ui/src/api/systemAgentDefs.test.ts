import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  listSystemAgentDefs,
  getSystemAgentDef,
  createSystemAgentDef,
  updateSystemAgentDef,
  deleteSystemAgentDef,
  type SystemAgentDef,
  type CreateSystemAgentDefRequest,
  type UpdateSystemAgentDefRequest,
} from './systemAgentDefs'
import * as client from './client'

vi.mock('./client')

function createMockAgent(overrides: Partial<SystemAgentDef> = {}): SystemAgentDef {
  return {
    id: 'conflict-resolver',
    model: 'sonnet',
    timeout: 30,
    prompt: 'Resolve merge conflicts in ${BRANCH_NAME}',
    restart_threshold: null,
    max_fail_restarts: null,
    stall_start_timeout_sec: null,
    stall_running_timeout_sec: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('listSystemAgentDefs', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls GET /api/v1/system-agents', async () => {
    const agents = [createMockAgent()]
    vi.mocked(client.apiGet).mockResolvedValue(agents)

    const result = await listSystemAgentDefs()

    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/system-agents')
    expect(result).toEqual(agents)
  })

  it('handles multiple agents', async () => {
    const agents = [
      createMockAgent({ id: 'conflict-resolver' }),
      createMockAgent({ id: 'code-reviewer' }),
    ]
    vi.mocked(client.apiGet).mockResolvedValue(agents)
    expect(await listSystemAgentDefs()).toHaveLength(2)
  })

  it('returns empty array', async () => {
    vi.mocked(client.apiGet).mockResolvedValue([])
    expect(await listSystemAgentDefs()).toEqual([])
  })

  it('propagates errors', async () => {
    vi.mocked(client.apiGet).mockRejectedValue(new Error('Network error'))
    await expect(listSystemAgentDefs()).rejects.toThrow('Network error')
  })
})

describe('getSystemAgentDef', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls GET with encoded ID', async () => {
    const agent = createMockAgent()
    vi.mocked(client.apiGet).mockResolvedValue(agent)

    await getSystemAgentDef('conflict-resolver')

    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/system-agents/conflict-resolver')
  })

  it('encodes special characters in ID', async () => {
    vi.mocked(client.apiGet).mockResolvedValue(createMockAgent())
    await getSystemAgentDef('my agent/v2')
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/system-agents/my%20agent%2Fv2')
  })

  it('propagates errors', async () => {
    vi.mocked(client.apiGet).mockRejectedValue(new Error('Not found'))
    await expect(getSystemAgentDef('missing')).rejects.toThrow('Not found')
  })
})

describe('createSystemAgentDef', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls POST /api/v1/system-agents with full request body', async () => {
    const req: CreateSystemAgentDefRequest = {
      id: 'conflict-resolver',
      model: 'sonnet',
      timeout: 30,
      prompt: 'Resolve merge conflicts',
      restart_threshold: null,
      max_fail_restarts: null,
      stall_start_timeout_sec: 120,
      stall_running_timeout_sec: null,
    }
    const created = createMockAgent()
    vi.mocked(client.apiPost).mockResolvedValue(created)

    const result = await createSystemAgentDef(req)

    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/system-agents', req)
    expect(result).toEqual(created)
  })

  it('calls POST with minimal required fields', async () => {
    vi.mocked(client.apiPost).mockResolvedValue(createMockAgent())
    await createSystemAgentDef({ id: 'x', prompt: 'p' })
    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/system-agents', { id: 'x', prompt: 'p' })
  })

  it('propagates errors', async () => {
    vi.mocked(client.apiPost).mockRejectedValue(new Error('Conflict'))
    await expect(createSystemAgentDef({ id: 'x', prompt: 'p' })).rejects.toThrow('Conflict')
  })
})

describe('updateSystemAgentDef', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls PATCH with encoded ID and body', async () => {
    const req: UpdateSystemAgentDefRequest = { model: 'opus', timeout: 60 }
    vi.mocked(client.apiPatch).mockResolvedValue({ status: 'ok' })

    const result = await updateSystemAgentDef('conflict-resolver', req)

    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/system-agents/conflict-resolver', req)
    expect(result).toEqual({ status: 'ok' })
  })

  it('encodes special characters in ID', async () => {
    vi.mocked(client.apiPatch).mockResolvedValue({ status: 'ok' })
    await updateSystemAgentDef('agent/v2', { prompt: 'updated' })
    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/system-agents/agent%2Fv2', { prompt: 'updated' })
  })

  it('sends null for cleared optional fields', async () => {
    vi.mocked(client.apiPatch).mockResolvedValue({ status: 'ok' })
    const req: UpdateSystemAgentDefRequest = { restart_threshold: null, stall_start_timeout_sec: null }
    await updateSystemAgentDef('conflict-resolver', req)
    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/system-agents/conflict-resolver', req)
  })

  it('propagates errors', async () => {
    vi.mocked(client.apiPatch).mockRejectedValue(new Error('Server error'))
    await expect(updateSystemAgentDef('x', {})).rejects.toThrow('Server error')
  })
})

describe('deleteSystemAgentDef', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls DELETE with encoded ID', async () => {
    vi.mocked(client.apiDelete).mockResolvedValue({ status: 'ok' })

    const result = await deleteSystemAgentDef('conflict-resolver')

    expect(client.apiDelete).toHaveBeenCalledWith('/api/v1/system-agents/conflict-resolver')
    expect(result).toEqual({ status: 'ok' })
  })

  it('encodes special characters in ID', async () => {
    vi.mocked(client.apiDelete).mockResolvedValue({ status: 'ok' })
    await deleteSystemAgentDef('agent/v2')
    expect(client.apiDelete).toHaveBeenCalledWith('/api/v1/system-agents/agent%2Fv2')
  })

  it('propagates errors', async () => {
    vi.mocked(client.apiDelete).mockRejectedValue(new Error('Not found'))
    await expect(deleteSystemAgentDef('missing')).rejects.toThrow('Not found')
  })
})
