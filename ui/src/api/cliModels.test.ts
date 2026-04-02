import { describe, it, expect, vi, beforeEach } from 'vitest'
import { testCLIModel } from './cliModels'
import * as client from './client'

vi.mock('./client')

describe('testCLIModel', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls POST /api/v1/cli-models/:id/test with empty body', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ success: true, duration_ms: 500 })
    const result = await testCLIModel('sonnet')
    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/cli-models/sonnet/test', {})
    expect(result).toEqual({ success: true, duration_ms: 500 })
  })

  it('encodes special characters in model id', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ success: false, error: 'err', duration_ms: 0 })
    await testCLIModel('model/with/slashes')
    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/cli-models/model%2Fwith%2Fslashes/test', {})
  })

  it('propagates errors', async () => {
    vi.mocked(client.apiPost).mockRejectedValue(new Error('timeout'))
    await expect(testCLIModel('sonnet')).rejects.toThrow('timeout')
  })
})
