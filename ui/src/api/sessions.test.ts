import { describe, it, expect, vi, beforeEach } from 'vitest'
import { listSessions, listGlobalSessions, getSessionFlow, getSessionStats } from './sessions'
import * as client from './client'

vi.mock('./client')

const emptyList = { sessions: [] }
const emptyFlow = { root_session_id: 'root', nodes: [], edges: [], truncated: false }
const emptyStats = {
  root_session_id: 'sid',
  tool_calls: [],
  self_cost_usd: 0,
  subtree_cost_usd: 0,
  self_tokens: 0,
  subtree_tokens: 0,
}

describe('sessions api', () => {
  beforeEach(() => vi.clearAllMocks())

  it('listSessions hits the project-scoped endpoint with no query params by default', async () => {
    vi.mocked(client.apiGet).mockResolvedValue(emptyList)
    await listSessions()
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/sessions')
  })

  it('listSessions includes limit in the query string', async () => {
    vi.mocked(client.apiGet).mockResolvedValue(emptyList)
    await listSessions({ limit: 50 })
    const url = vi.mocked(client.apiGet).mock.calls[0][0] as string
    expect(url).toContain('/api/v1/sessions?')
    expect(url).toContain('limit=50')
  })

  it('listGlobalSessions hits the /global endpoint', async () => {
    vi.mocked(client.apiGet).mockResolvedValue(emptyList)
    await listGlobalSessions({ limit: 10 })
    const url = vi.mocked(client.apiGet).mock.calls[0][0] as string
    expect(url).toContain('/api/v1/sessions/global')
    expect(url).toContain('limit=10')
  })

  it('getSessionFlow encodes the session id into the path', async () => {
    vi.mocked(client.apiGet).mockResolvedValue(emptyFlow)
    await getSessionFlow('sid with spaces')
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/sessions/sid%20with%20spaces/flow')
  })

  it('getSessionStats encodes the session id into the path', async () => {
    vi.mocked(client.apiGet).mockResolvedValue(emptyStats)
    await getSessionStats('sid/with/slash')
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/sessions/sid%2Fwith%2Fslash/stats')
  })
})
