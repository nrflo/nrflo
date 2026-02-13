import { describe, it, expect, vi, beforeEach } from 'vitest'
import { retryFailedAgent } from './workflows'
import * as client from './client'

vi.mock('./client', () => ({
  apiPost: vi.fn(),
}))

describe('workflows API - retryFailedAgent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls POST /api/v1/tickets/:id/workflow/retry-failed with correct parameters', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ status: 'retrying' })

    await retryFailedAgent('TICKET-123', {
      workflow: 'feature',
      session_id: 'sess-abc-123',
    })

    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/tickets/TICKET-123/workflow/retry-failed',
      {
        workflow: 'feature',
        session_id: 'sess-abc-123',
      }
    )
  })

  it('URL-encodes ticket ID', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ status: 'retrying' })

    await retryFailedAgent('TICKET-WITH-SPECIAL/CHARS', {
      workflow: 'bugfix',
      session_id: 'sess-xyz',
    })

    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/tickets/TICKET-WITH-SPECIAL%2FCHARS/workflow/retry-failed',
      {
        workflow: 'bugfix',
        session_id: 'sess-xyz',
      }
    )
  })

  it('returns response from server', async () => {
    const expectedResponse = { status: 'retrying', message: 'Retry initiated' }
    vi.mocked(client.apiPost).mockResolvedValue(expectedResponse)

    const result = await retryFailedAgent('TICKET-1', {
      workflow: 'feature',
      session_id: 'sess-1',
    })

    expect(result).toEqual(expectedResponse)
  })

  it('propagates errors from API client', async () => {
    const error = new Error('Network error')
    vi.mocked(client.apiPost).mockRejectedValue(error)

    await expect(
      retryFailedAgent('TICKET-1', {
        workflow: 'feature',
        session_id: 'sess-1',
      })
    ).rejects.toThrow('Network error')
  })

  it('sends workflow and session_id in request body', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ status: 'retrying' })

    await retryFailedAgent('TICKET-999', {
      workflow: 'hotfix',
      session_id: 'sess-urgent-123',
    })

    const [, body] = vi.mocked(client.apiPost).mock.calls[0]
    expect(body).toEqual({
      workflow: 'hotfix',
      session_id: 'sess-urgent-123',
    })
  })
})
