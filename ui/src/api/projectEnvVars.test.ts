import { describe, it, expect, vi, beforeEach } from 'vitest'
import * as client from './client'

vi.mock('./client')

import { listEnvVars, putEnvVar, deleteEnvVar } from './projectEnvVars'

beforeEach(() => vi.clearAllMocks())

describe('projectEnvVars API', () => {
  describe('listEnvVars', () => {
    it('calls GET /api/v1/projects/:id/env-vars', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await listEnvVars('proj-1')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/projects/proj-1/env-vars')
    })

    it('encodes special characters in projectId', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await listEnvVars('my project/id')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/projects/my%20project%2Fid/env-vars')
    })
  })

  describe('putEnvVar', () => {
    it('calls PUT /api/v1/projects/:id/env-vars/:name with value body', async () => {
      vi.mocked(client.apiPut).mockResolvedValue({})
      await putEnvVar('proj-1', 'API_KEY', 'secret')
      expect(client.apiPut).toHaveBeenCalledWith(
        '/api/v1/projects/proj-1/env-vars/API_KEY',
        { value: 'secret' }
      )
    })

    it('encodes special characters in name', async () => {
      vi.mocked(client.apiPut).mockResolvedValue({})
      await putEnvVar('proj-1', 'MY VAR/KEY', 'val')
      expect(client.apiPut).toHaveBeenCalledWith(
        '/api/v1/projects/proj-1/env-vars/MY%20VAR%2FKEY',
        { value: 'val' }
      )
    })

    it('encodes special characters in projectId', async () => {
      vi.mocked(client.apiPut).mockResolvedValue({})
      await putEnvVar('my/proj', 'KEY', 'val')
      expect(client.apiPut).toHaveBeenCalledWith(
        '/api/v1/projects/my%2Fproj/env-vars/KEY',
        { value: 'val' }
      )
    })
  })

  describe('deleteEnvVar', () => {
    it('calls DELETE /api/v1/projects/:id/env-vars/:name', async () => {
      vi.mocked(client.apiDelete).mockResolvedValue(undefined)
      await deleteEnvVar('proj-1', 'API_KEY')
      expect(client.apiDelete).toHaveBeenCalledWith('/api/v1/projects/proj-1/env-vars/API_KEY')
    })

    it('encodes special characters in name', async () => {
      vi.mocked(client.apiDelete).mockResolvedValue(undefined)
      await deleteEnvVar('proj-1', 'MY KEY/NAME')
      expect(client.apiDelete).toHaveBeenCalledWith(
        '/api/v1/projects/proj-1/env-vars/MY%20KEY%2FNAME'
      )
    })
  })
})
