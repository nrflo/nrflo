import { describe, it, expect, vi, beforeEach } from 'vitest'
import * as client from './client'

vi.mock('./client')

import {
  getSummary,
  getEditRate,
  getThroughput,
} from './insights'

beforeEach(() => vi.clearAllMocks())

describe('insights API', () => {
  describe('getSummary', () => {
    it('calls GET /api/v1/insights/summary?range=7d', async () => {
      vi.mocked(client.apiGet).mockResolvedValue({})
      await getSummary('7d')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/insights/summary?range=7d')
    })
  })

  describe('getEditRate', () => {
    it('calls GET /api/v1/insights/edit-rate?range=30d', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await getEditRate('30d')
      expect(client.apiGet).toHaveBeenCalledWith('/api/v1/insights/edit-rate?range=30d')
    })
  })

  describe('getThroughput', () => {
    it('calls GET with range and bucket params', async () => {
      vi.mocked(client.apiGet).mockResolvedValue([])
      await getThroughput('7d', '1h')
      expect(client.apiGet).toHaveBeenCalledWith(
        '/api/v1/insights/throughput?range=7d&bucket=1h'
      )
    })
  })
})
