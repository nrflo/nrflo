import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { dispatchV2Event, clearSeqs } from './useWSReducer'
import { sessionKeys } from './useSessions'
import type { WSEventV2 } from './useWSProtocol'

describe('useWSReducer — sessionKeys.all invalidation', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    clearSeqs()
    sessionStorage.clear()
  })

  afterEach(() => {
    clearSeqs()
    sessionStorage.clear()
  })

  function makeEvent(type: string, seq = 1): WSEventV2 {
    return {
      type,
      project_id: 'proj1',
      ticket_id: 'T-1',
      timestamp: '2026-01-01T00:00:00Z',
      sequence: seq,
      protocol_version: 2,
    }
  }

  it.each([
    'orchestration.started',
    'orchestration.completed',
    'orchestration.failed',
    'orchestration.retried',
    'orchestration.callback',
    'agent.completed',
  ])('%s invalidates sessionKeys.all exactly once', (type) => {
    const spy = vi.spyOn(queryClient, 'invalidateQueries')
    dispatchV2Event(makeEvent(type), queryClient)
    const calls = spy.mock.calls.filter(([arg]) => JSON.stringify(arg?.queryKey) === JSON.stringify(sessionKeys.all))
    expect(calls).toHaveLength(1)
  })

  it.each(['messages.updated', 'workflow.updated'])(
    '%s does not invalidate sessionKeys.all',
    (type) => {
      const spy = vi.spyOn(queryClient, 'invalidateQueries')
      dispatchV2Event(makeEvent(type), queryClient)
      const calls = spy.mock.calls.filter(
        ([arg]) => JSON.stringify(arg?.queryKey) === JSON.stringify(sessionKeys.all)
      )
      expect(calls).toHaveLength(0)
    }
  )
})
