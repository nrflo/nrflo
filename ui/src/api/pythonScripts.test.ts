import { describe, it, expect, vi, beforeEach } from 'vitest'
import * as client from './client'

vi.mock('./client')

import {
  listPythonScripts,
  getPythonScript,
  createPythonScript,
  updatePythonScript,
  deletePythonScript,
  validatePythonScript,
} from './pythonScripts'

beforeEach(() => vi.clearAllMocks())

describe('pythonScripts API', () => {
  describe('listPythonScripts', () => {
    it('calls GET /api/v1/python-scripts', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await listPythonScripts()
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/python-scripts')
    })
  })

  describe('getPythonScript', () => {
    it('calls GET /api/v1/python-scripts/:id', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getPythonScript('script-123')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/python-scripts/script-123')
    })

    it('encodes special characters in id', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getPythonScript('a/b c')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/python-scripts/a%2Fb%20c')
    })
  })

  describe('createPythonScript', () => {
    it('calls POST /api/v1/python-scripts with data', async () => {
      vi.mocked(client.apiPost).mockResolvedValue({})
      const data = { name: 'test', code: 'print()', description: '' }
      await createPythonScript(data)
      expect(client.apiPost).toHaveBeenCalledWith('/api/v1/python-scripts', data)
    })
  })

  describe('updatePythonScript', () => {
    it('calls PATCH /api/v1/python-scripts/:id with data', async () => {
      vi.mocked(client.apiPatch).mockResolvedValue({ status: 'ok' })
      const data = { name: 'renamed', code: 'pass', description: 'desc' }
      await updatePythonScript('s1', data)
      expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/python-scripts/s1', data)
    })

    it('encodes special characters in id', async () => {
      vi.mocked(client.apiPatch).mockResolvedValue({ status: 'ok' })
      await updatePythonScript('a/b', { name: 'x', code: '', description: '' })
      expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/python-scripts/a%2Fb', expect.any(Object))
    })
  })

  describe('deletePythonScript', () => {
    it('calls DELETE /api/v1/python-scripts/:id', async () => {
      vi.mocked(client.apiDelete).mockResolvedValue({ status: 'ok' })
      await deletePythonScript('del-1')
      expect(client.apiDelete).toHaveBeenCalledWith('/api/v1/python-scripts/del-1')
    })
  })

  describe('validatePythonScript', () => {
    it('calls POST /api/v1/python-scripts/validate with code in body', async () => {
      vi.mocked(client.apiPost).mockResolvedValue({ ok: true })
      await validatePythonScript('print("hello")')
      expect(client.apiPost).toHaveBeenCalledWith(
        '/api/v1/python-scripts/validate',
        { code: 'print("hello")' }
      )
    })
  })
})
