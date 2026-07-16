import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as client from './client'
import { createModel, deleteModel, getModel, listModels, testModel, updateModel } from './models'

vi.mock('./client')

describe('models client', () => {
  beforeEach(() => vi.clearAllMocks())

  it('uses the unified CRUD endpoints', async () => {
    vi.mocked(client.apiGet).mockResolvedValue([])
    vi.mocked(client.apiPost).mockResolvedValue({})
    vi.mocked(client.apiPatch).mockResolvedValue({})
    vi.mocked(client.apiDelete).mockResolvedValue({ status: 'deleted' })
    const request = {
      id: 'custom/model', provider: 'openai' as const, display_name: 'Custom', cli_model: 'custom', api_model: '',
      cli_efforts: ['low'], api_efforts: [], cli_context: 200000, api_context: 200000,
      fallback_models: '', default_effort: 'low',
    }

    await listModels()
    await getModel('custom/model')
    await createModel(request)
    await updateModel('custom/model', { enabled: false })
    await deleteModel('custom/model')

    expect(client.apiGet).toHaveBeenNthCalledWith(1, '/api/v1/models')
    expect(client.apiGet).toHaveBeenNthCalledWith(2, '/api/v1/models/custom%2Fmodel')
    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/models', request)
    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/models/custom%2Fmodel', { enabled: false })
    expect(client.apiDelete).toHaveBeenCalledWith('/api/v1/models/custom%2Fmodel')
  })

  it('tests a model through its CLI probe endpoint', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ success: true, duration_ms: 12 })
    const controller = new AbortController()
    await testModel('sonnet-5', controller.signal)
    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/models/sonnet-5/test', {}, { signal: controller.signal })
  })
})
