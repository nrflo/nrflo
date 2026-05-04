import { describe, it, expect, vi, beforeEach } from 'vitest'
import * as client from './client'

vi.mock('./client')

import {
  listReviewItems,
  getReviewItem,
  updateReviewDraft,
  approveReview,
  rejectReview,
} from './review'

beforeEach(() => vi.clearAllMocks())

describe('review API', () => {
  describe('listReviewItems', () => {
    it('calls GET /api/v1/review with no query string when no params', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await listReviewItems()
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/review')
    })

    it('appends status param', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await listReviewItems({ status: 'pending' })
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/review?status=pending')
    })

    it('appends limit and offset params', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await listReviewItems({ limit: 10, offset: 20 })
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/review?limit=10&offset=20')
    })
  })

  describe('getReviewItem', () => {
    it('calls GET /api/v1/review/:id', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getReviewItem('item-1')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/review/item-1')
    })

    it('encodes special chars in id', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getReviewItem('item/with spaces')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/review/item%2Fwith%20spaces')
    })
  })

  describe('updateReviewDraft', () => {
    it('calls PATCH /api/v1/review/:id with draft body', async () => {
      vi.mocked(client.apiPatch).mockResolvedValue({})
      await updateReviewDraft('item-1', { key: 'value' })
      expect(client.apiPatch).toHaveBeenCalledWith(
        '/api/v1/review/item-1',
        { draft: { key: 'value' } }
      )
    })
  })

  describe('approveReview', () => {
    it('calls POST /api/v1/review/:id/approve', async () => {
      vi.mocked(client.apiPost).mockResolvedValue({})
      await approveReview('item-2')
      expect(client.apiPost).toHaveBeenCalledWith('/api/v1/review/item-2/approve')
    })
  })

  describe('rejectReview', () => {
    it('calls POST /api/v1/review/:id/reject with reason', async () => {
      vi.mocked(client.apiPost).mockResolvedValue({})
      await rejectReview('item-3', 'not good')
      expect(client.apiPost).toHaveBeenCalledWith(
        '/api/v1/review/item-3/reject',
        { reason: 'not good' }
      )
    })
  })
})
