import { describe, it, expect, vi, beforeEach } from 'vitest'
import { retryFailedProjectAgent } from './projectWorkflows'
import * as client from './client'

vi.mock('./client', () => ({
  apiPost: vi.fn(),
}))

describe('projectWorkflows API - retryFailedProjectAgent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls POST /api/v1/projects/:id/workflow/retry-failed with correct parameters', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ status: 'retrying' })

    await retryFailedProjectAgent('test-project', {
      workflow: 'feature',
      session_id: 'sess-project-123',
    })

    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/projects/test-project/workflow/retry-failed',
      {
        workflow: 'feature',
        session_id: 'sess-project-123',
      }
    )
  })

  it('URL-encodes project ID', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ status: 'retrying' })

    await retryFailedProjectAgent('project/with/slashes', {
      workflow: 'bugfix',
      session_id: 'sess-xyz',
    })

    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/projects/project%2Fwith%2Fslashes/workflow/retry-failed',
      {
        workflow: 'bugfix',
        session_id: 'sess-xyz',
      }
    )
  })

  it('returns response from server', async () => {
    const expectedResponse = { status: 'retrying', message: 'Project retry initiated' }
    vi.mocked(client.apiPost).mockResolvedValue(expectedResponse)

    const result = await retryFailedProjectAgent('my-project', {
      workflow: 'feature',
      session_id: 'sess-1',
    })

    expect(result).toEqual(expectedResponse)
  })

  it('propagates errors from API client', async () => {
    const error = new Error('API error')
    vi.mocked(client.apiPost).mockRejectedValue(error)

    await expect(
      retryFailedProjectAgent('my-project', {
        workflow: 'feature',
        session_id: 'sess-1',
      })
    ).rejects.toThrow('API error')
  })

  it('sends workflow and session_id in request body', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ status: 'retrying' })

    await retryFailedProjectAgent('prod-project', {
      workflow: 'hotfix',
      session_id: 'sess-urgent-999',
    })

    const [, body] = vi.mocked(client.apiPost).mock.calls[0]
    expect(body).toEqual({
      workflow: 'hotfix',
      session_id: 'sess-urgent-999',
    })
  })
})
