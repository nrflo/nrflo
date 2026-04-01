import { describe, it, expect, vi, beforeEach } from 'vitest'
import { listTickets } from './tickets'
import * as client from './client'

vi.mock('./client')

describe('listTickets', () => {
  beforeEach(() => vi.clearAllMocks())

  it('builds URL with page and per_page params', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({
      tickets: [], total_count: 0, page: 1, per_page: 10, total_pages: 0,
    })
    await listTickets({ page: 2, per_page: 10 })
    expect(client.apiGet).toHaveBeenCalledWith(
      expect.stringContaining('page=2')
    )
    expect(client.apiGet).toHaveBeenCalledWith(
      expect.stringContaining('per_page=10')
    )
  })

  it('includes sort_by and sort_order in query string', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({
      tickets: [], total_count: 0, page: 1, per_page: 30, total_pages: 0,
    })
    await listTickets({ sort_by: 'priority', sort_order: 'asc' })
    expect(client.apiGet).toHaveBeenCalledWith(
      expect.stringContaining('sort_by=priority')
    )
    expect(client.apiGet).toHaveBeenCalledWith(
      expect.stringContaining('sort_order=asc')
    )
  })

  it('omits undefined params from query string', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({
      tickets: [], total_count: 0, page: 1, per_page: 30, total_pages: 0,
    })
    await listTickets({})
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/tickets')
  })

  it('includes all params together in correct URL', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({
      tickets: [], total_count: 100, page: 2, per_page: 10, total_pages: 10,
    })
    await listTickets({ status: 'open', type: 'bug', page: 2, per_page: 10, sort_by: 'created_at', sort_order: 'asc' })
    const url = vi.mocked(client.apiGet).mock.calls[0][0] as string
    expect(url).toContain('status=open')
    expect(url).toContain('type=bug')
    expect(url).toContain('page=2')
    expect(url).toContain('per_page=10')
    expect(url).toContain('sort_by=created_at')
    expect(url).toContain('sort_order=asc')
  })
})
