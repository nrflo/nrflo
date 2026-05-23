import { describe, it, expect, vi, beforeEach } from 'vitest'
import { listAPIModels, getAPIModel, createAPIModel, updateAPIModel, deleteAPIModel } from './apiModels'
import * as client from './client'

vi.mock('./client')

describe('apiModels client', () => {
  beforeEach(() => vi.clearAllMocks())

  it('listAPIModels calls GET /api/v1/api-models', async () => {
    vi.mocked(client.apiGet).mockResolvedValue([])
    await listAPIModels()
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/api-models')
  })

  it('getAPIModel calls GET /api/v1/api-models/:id with encoding', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({ id: 'test' })
    await getAPIModel('model/with/slashes')
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/api-models/model%2Fwith%2Fslashes')
  })

  it('createAPIModel calls POST /api/v1/api-models', async () => {
    const req = { id: 'my-model', provider: 'anthropic' as const, display_name: 'My Model', mapped_model: 'claude-opus-4-7' }
    vi.mocked(client.apiPost).mockResolvedValue({ id: 'my-model' })
    await createAPIModel(req)
    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/api-models', req)
  })

  it('updateAPIModel calls PATCH /api/v1/api-models/:id with encoding', async () => {
    vi.mocked(client.apiPatch).mockResolvedValue({ status: 'ok' })
    await updateAPIModel('my model', { enabled: false })
    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/api-models/my%20model', { enabled: false })
  })

  it('deleteAPIModel calls DELETE /api/v1/api-models/:id with encoding', async () => {
    vi.mocked(client.apiDelete).mockResolvedValue({ status: 'ok' })
    await deleteAPIModel('my/model')
    expect(client.apiDelete).toHaveBeenCalledWith('/api/v1/api-models/my%2Fmodel')
  })

  it('propagates errors from the client', async () => {
    vi.mocked(client.apiGet).mockRejectedValue(new Error('network error'))
    await expect(listAPIModels()).rejects.toThrow('network error')
  })
})
