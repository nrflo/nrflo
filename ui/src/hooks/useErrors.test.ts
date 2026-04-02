import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useErrors, errorKeys } from './useErrors'
import * as errorsApi from '@/api/errors'
import { createTestQueryClient, createWrapper } from '@/test/utils'
import type { ErrorsResponse } from '@/types/errors'

vi.mock('@/api/errors')

const mockProjectStore = vi.fn((selector: (s: any) => any) =>
  selector({ currentProject: 'proj-1', projectsLoaded: true })
)

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: any) => mockProjectStore(selector),
}))

function makeResponse(overrides: Partial<ErrorsResponse> = {}): ErrorsResponse {
  return {
    errors: [],
    total: 0,
    page: 1,
    per_page: 20,
    total_pages: 1,
    ...overrides,
  }
}

describe('errorKeys', () => {
  it('all is ["errors"]', () => {
    expect(errorKeys.all).toEqual(['errors'])
  })

  it('lists() is ["errors", "list"]', () => {
    expect(errorKeys.lists()).toEqual(['errors', 'list'])
  })

  it('list(params) embeds params in key', () => {
    const key = errorKeys.list({ page: 2, type: 'agent' })
    expect(key).toEqual(['errors', 'list', { page: 2, type: 'agent' }])
  })
})

describe('useErrors', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockProjectStore.mockImplementation((selector: any) =>
      selector({ currentProject: 'proj-1', projectsLoaded: true })
    )
  })

  it('fetches errors and returns data', async () => {
    const response = makeResponse({ total: 5 })
    vi.mocked(errorsApi.fetchErrors).mockResolvedValue(response)

    const { result } = renderHook(
      () => useErrors({ page: 1, perPage: 20 }),
      { wrapper: createWrapper(createTestQueryClient()) }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual(response)
    expect(errorsApi.fetchErrors).toHaveBeenCalledWith({ page: 1, perPage: 20 })
  })

  it('is disabled when projectsLoaded is false', () => {
    mockProjectStore.mockImplementation((selector: any) =>
      selector({ currentProject: 'proj-1', projectsLoaded: false })
    )

    const { result } = renderHook(
      () => useErrors(),
      { wrapper: createWrapper(createTestQueryClient()) }
    )

    expect(result.current.fetchStatus).toBe('idle')
    expect(errorsApi.fetchErrors).not.toHaveBeenCalled()
  })

  it('query key includes project for cache isolation', async () => {
    vi.mocked(errorsApi.fetchErrors).mockResolvedValue(makeResponse())
    const qc = createTestQueryClient()

    const { result } = renderHook(
      () => useErrors({ type: 'agent' }),
      { wrapper: createWrapper(qc) }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    // Verify the query key in cache includes the project
    const queries = qc.getQueryCache().getAll()
    expect(queries.some((q) =>
      JSON.stringify(q.queryKey).includes('proj-1') &&
      JSON.stringify(q.queryKey).includes('errors')
    )).toBe(true)
  })
})
