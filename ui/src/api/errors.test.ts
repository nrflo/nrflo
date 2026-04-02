import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fetchErrors } from './errors'
import * as client from './client'

vi.mock('./client')

describe('fetchErrors', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls GET /api/v1/errors with no query string when no params', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({ errors: [], total: 0, page: 1, per_page: 20, total_pages: 1 })
    await fetchErrors()
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/errors')
  })

  it('appends page param', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({ errors: [], total: 0, page: 2, per_page: 20, total_pages: 3 })
    await fetchErrors({ page: 2 })
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/errors?page=2')
  })

  it('appends per_page from perPage field', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({ errors: [], total: 0, page: 1, per_page: 10, total_pages: 1 })
    await fetchErrors({ perPage: 10 })
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/errors?per_page=10')
  })

  it('appends type param', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({ errors: [], total: 0, page: 1, per_page: 20, total_pages: 1 })
    await fetchErrors({ type: 'agent' })
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/errors?type=agent')
  })

  it('combines page, perPage, and type params', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({ errors: [], total: 0, page: 2, per_page: 20, total_pages: 3 })
    await fetchErrors({ page: 2, perPage: 20, type: 'workflow' })
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/errors?page=2&per_page=20&type=workflow')
  })

  it('returns the API response', async () => {
    const response = { errors: [], total: 5, page: 1, per_page: 20, total_pages: 1 }
    vi.mocked(client.apiGet).mockResolvedValue(response)
    const result = await fetchErrors()
    expect(result).toEqual(response)
  })
})
