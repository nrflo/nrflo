import { describe, it, expect, vi, beforeEach } from 'vitest'
import * as client from './client'

vi.mock('./client')

import {
  listConfigFiles,
  getConfigFile,
  putConfigFile,
  getConfigHistory,
  rollbackConfig,
} from './configFiles'

beforeEach(() => vi.clearAllMocks())

describe('configFiles API', () => {
  describe('listConfigFiles', () => {
    it('calls GET /api/v1/config-files', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await listConfigFiles()
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/config-files')
    })
  })

  describe('getConfigFile', () => {
    it('calls GET /api/v1/config-files/content/:path with encoded segments', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getConfigFile('dir/file.yaml')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/config-files/content/dir/file.yaml')
    })

    it('encodes path segments individually', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getConfigFile('dir with spaces/file name.yaml')
      expect(client.apiGet).toHaveBeenCalledWith(
        '/api/v1/config-files/content/dir%20with%20spaces/file%20name.yaml'
      )
    })
  })

  describe('putConfigFile', () => {
    it('calls apiFetch PUT with text/plain content-type', async () => {
      vi.mocked(client.apiFetch).mockResolvedValue({})
      await putConfigFile('dir/file.yaml', 'content here')
      expect(client.apiFetch).toHaveBeenCalledWith(
        '/api/v1/config-files/content/dir/file.yaml',
        expect.objectContaining({
          method: 'PUT',
          headers: { 'Content-Type': 'text/plain' },
          body: 'content here',
        })
      )
    })
  })

  describe('getConfigHistory', () => {
    it('calls GET /api/v1/config-files/history/:path', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await getConfigHistory('dir/file.yaml')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/config-files/history/dir/file.yaml')
    })
  })

  describe('rollbackConfig', () => {
    it('calls POST /api/v1/config-files/rollback/:path with version', async () => {
      vi.mocked(client.apiPost).mockResolvedValue({})
      await rollbackConfig('dir/file.yaml', 3)
      expect(client.apiPost).toHaveBeenCalledWith(
        '/api/v1/config-files/rollback/dir/file.yaml',
        { version: 3 }
      )
    })
  })
})
