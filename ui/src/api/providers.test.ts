import { describe, it, expect, vi, beforeEach } from 'vitest'
import { listProviders, updateProvider } from './providers'
import * as client from './client'

vi.mock('./client')

describe('providers API', () => {
  beforeEach(() => vi.clearAllMocks())

  describe('listProviders', () => {
    it('calls GET /api/v1/providers and returns response', async () => {
      const data = { claude: { modes: ['cli', 'cli_interactive'] } }
      vi.mocked(client.apiGet).mockResolvedValue(data)
      const result = await listProviders()
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/providers')
      expect(result).toEqual(data)
    })

    it('propagates errors from apiGet', async () => {
      vi.mocked(client.apiGet).mockRejectedValue(new Error('network'))
      await expect(listProviders()).rejects.toThrow('network')
    })
  })

  describe('updateProvider', () => {
    it('calls PATCH /api/v1/providers/:name with modes payload', async () => {
      vi.mocked(client.apiPatch).mockResolvedValue({ status: 'ok' })
      const result = await updateProvider('claude', ['cli'])
      expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/providers/claude', { modes: ['cli'] })
      expect(result).toEqual({ status: 'ok' })
    })

    it('encodes special characters in provider name', async () => {
      vi.mocked(client.apiPatch).mockResolvedValue({ status: 'ok' })
      await updateProvider('open/code' as never, ['cli'])
      expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/providers/open%2Fcode', { modes: ['cli'] })
    })

    it('propagates errors from apiPatch', async () => {
      vi.mocked(client.apiPatch).mockRejectedValue(new Error('server error'))
      await expect(updateProvider('claude', ['cli'])).rejects.toThrow('server error')
    })
  })
})
