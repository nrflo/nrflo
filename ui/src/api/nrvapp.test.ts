import { describe, it, expect, vi, beforeEach } from 'vitest'
import * as client from './client'

vi.mock('./client')

import {
  listReviewItems,
  getReviewItem,
  updateReviewDraft,
  approveReview,
  rejectReview,
  listConfigFiles,
  getConfigFile,
  putConfigFile,
  getConfigHistory,
  rollbackConfig,
  getSummary,
  getEditRate,
  getThroughput,
} from './nrvapp'

beforeEach(() => vi.clearAllMocks())

describe('nrvapp API', () => {
  describe('listReviewItems', () => {
    it('calls GET /api/v1/nrvapp/review with no query string when no params', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await listReviewItems()
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/nrvapp/review')
    })

    it('appends status param', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await listReviewItems({ status: 'pending' })
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/nrvapp/review?status=pending')
    })

    it('appends limit and offset params', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await listReviewItems({ limit: 10, offset: 20 })
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/nrvapp/review?limit=10&offset=20')
    })
  })

  describe('getReviewItem', () => {
    it('calls GET /api/v1/nrvapp/review/:id', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getReviewItem('item-1')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/nrvapp/review/item-1')
    })

    it('encodes special chars in id', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getReviewItem('item/with spaces')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/nrvapp/review/item%2Fwith%20spaces')
    })
  })

  describe('updateReviewDraft', () => {
    it('calls PATCH /api/v1/nrvapp/review/:id with draft body', async () => {
      vi.mocked(client.apiPatch).mockResolvedValue({})
      await updateReviewDraft('item-1', { key: 'value' })
      expect(client.apiPatch).toHaveBeenCalledWith(
        '/api/v1/nrvapp/review/item-1',
        { draft: { key: 'value' } }
      )
    })
  })

  describe('approveReview', () => {
    it('calls POST /api/v1/nrvapp/review/:id/approve', async () => {
      vi.mocked(client.apiPost).mockResolvedValue({})
      await approveReview('item-2')
      expect(client.apiPost).toHaveBeenCalledWith('/api/v1/nrvapp/review/item-2/approve')
    })
  })

  describe('rejectReview', () => {
    it('calls POST /api/v1/nrvapp/review/:id/reject with reason', async () => {
      vi.mocked(client.apiPost).mockResolvedValue({})
      await rejectReview('item-3', 'not good')
      expect(client.apiPost).toHaveBeenCalledWith(
        '/api/v1/nrvapp/review/item-3/reject',
        { reason: 'not good' }
      )
    })
  })

  describe('listConfigFiles', () => {
    it('calls GET /api/v1/nrvapp/config/files', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await listConfigFiles()
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/nrvapp/config/files')
    })
  })

  describe('getConfigFile', () => {
    it('calls GET /api/v1/nrvapp/config/content/:path with encoded segments', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getConfigFile('dir/file.yaml')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/nrvapp/config/content/dir/file.yaml')
    })

    it('encodes path segments individually', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getConfigFile('dir with spaces/file name.yaml')
      expect(client.apiGet).toHaveBeenCalledWith(
        '/api/v1/nrvapp/config/content/dir%20with%20spaces/file%20name.yaml'
      )
    })
  })

  describe('putConfigFile', () => {
    it('calls apiFetch PUT with text/plain content-type', async () => {
      vi.mocked(client.apiFetch).mockResolvedValue({})
      await putConfigFile('dir/file.yaml', 'content here')
      expect(client.apiFetch).toHaveBeenCalledWith(
        '/api/v1/nrvapp/config/content/dir/file.yaml',
        expect.objectContaining({
          method: 'PUT',
          headers: { 'Content-Type': 'text/plain' },
          body: 'content here',
        })
      )
    })
  })

  describe('getConfigHistory', () => {
    it('calls GET /api/v1/nrvapp/config/history/:path', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await getConfigHistory('dir/file.yaml')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/nrvapp/config/history/dir/file.yaml')
    })
  })

  describe('rollbackConfig', () => {
    it('calls POST /api/v1/nrvapp/config/rollback/:path with version', async () => {
      vi.mocked(client.apiPost).mockResolvedValue({})
      await rollbackConfig('dir/file.yaml', 3)
      expect(client.apiPost).toHaveBeenCalledWith(
        '/api/v1/nrvapp/config/rollback/dir/file.yaml',
        { version: 3 }
      )
    })
  })

  describe('getSummary', () => {
    it('calls GET /api/v1/nrvapp/insights/summary?range=7d', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getSummary('7d')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/nrvapp/insights/summary?range=7d')
    })
  })

  describe('getEditRate', () => {
    it('calls GET /api/v1/nrvapp/insights/edit-rate?range=30d', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await getEditRate('30d')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/nrvapp/insights/edit-rate?range=30d')
    })
  })

  describe('getThroughput', () => {
    it('calls GET with range and bucket params', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await getThroughput('7d', '1h')
      expect(client.apiGet).toHaveBeenCalledWith(
        '/api/v1/nrvapp/insights/throughput?range=7d&bucket=1h'
      )
    })
  })
})
