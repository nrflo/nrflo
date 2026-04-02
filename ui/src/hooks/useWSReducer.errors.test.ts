import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import { errorKeys } from './useErrors'
import type { WSEventV2 } from './useWSProtocol'

describe('useWSReducer - error.created', () => {
  let queryClient: QueryClient
  let spy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    spy = vi.spyOn(queryClient, 'invalidateQueries')
    clearSeqs()
    sessionStorage.clear()
  })

  afterEach(() => {
    clearSeqs()
  })

  it('invalidates errorKeys.all when error.created event is received', () => {
    const event: WSEventV2 = {
      type: 'error.created',
      project_id: 'proj1',
      ticket_id: '',
      timestamp: '2026-01-01T00:00:00Z',
      sequence: 1,
      protocol_version: 2,
    }

    dispatchV2Event(event, queryClient)

    const expectedKey = JSON.stringify(errorKeys.all)
    expect(
      spy.mock.calls.some((call: any) =>
        JSON.stringify(call[0].queryKey) === expectedKey
      )
    ).toBe(true)
  })

  it('only invalidates once per unique sequence', () => {
    const event: WSEventV2 = {
      type: 'error.created',
      project_id: 'proj1',
      ticket_id: '',
      timestamp: '2026-01-01T00:00:00Z',
      sequence: 5,
      protocol_version: 2,
    }

    dispatchV2Event(event, queryClient)
    dispatchV2Event(event, queryClient) // duplicate seq — should be ignored

    const errorKeyStr = JSON.stringify(errorKeys.all)
    const matchingCalls = spy.mock.calls.filter((call: any) =>
      JSON.stringify(call[0].queryKey) === errorKeyStr
    )
    expect(matchingCalls).toHaveLength(1)
  })
})
