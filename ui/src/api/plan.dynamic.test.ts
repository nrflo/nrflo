import { describe, it, expect, vi, beforeEach } from 'vitest'
import { startDynamicWorkflow } from './plan'
import * as client from './client'

vi.mock('./client')

describe('plan api — startDynamicWorkflow', () => {
  beforeEach(() => vi.clearAllMocks())

  it('posts to /api/v1/projects/{id}/dynamic-workflow with the request body', async () => {
    const response = { instance_id: 'inst-9', status: 'planning' }
    vi.mocked(client.apiPost).mockResolvedValue(response)

    const req = { instructions: 'Build the thing', mode: 'approve' as const }
    const result = await startDynamicWorkflow('proj-1', req)

    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/projects/proj-1/dynamic-workflow',
      req
    )
    expect(result).toEqual(response)
  })

  it('encodeURIComponent-encodes the project id', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ instance_id: 'inst-9', status: 'planning' })
    await startDynamicWorkflow('proj/with space', { instructions: 'Go' })
    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/projects/proj%2Fwith%20space/dynamic-workflow',
      { instructions: 'Go' }
    )
  })

  it('supports mode: auto', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ instance_id: 'inst-9', status: 'planning', session_id: 'sess-1' })
    await startDynamicWorkflow('proj-1', { instructions: 'Go', mode: 'auto' })
    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/projects/proj-1/dynamic-workflow',
      { instructions: 'Go', mode: 'auto' }
    )
  })
})
