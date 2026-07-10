import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { dispatchV2Event, clearSeqs } from './useWSReducer'
import { traceKeys } from './useTrace'
import type { WSEventV2 } from './useWSProtocol'

describe('useWSReducer — trace invalidation', () => {
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

  it.each(['agent.started', 'agent.completed', 'workflow.updated'])(
    '%s invalidates traceKeys.all',
    (type) => {
      const spy = vi.spyOn(queryClient, 'invalidateQueries')
      dispatchV2Event(makeEvent(type), queryClient)
      expect(spy).toHaveBeenCalledWith(expect.objectContaining({ queryKey: traceKeys.all }))
    }
  )

  it('messages.updated does NOT invalidate traceKeys (component-level throttle instead)', () => {
    const spy = vi.spyOn(queryClient, 'invalidateQueries')
    dispatchV2Event(makeEvent('messages.updated'), queryClient)
    const traceCalls = spy.mock.calls.filter(([arg]) =>
      JSON.stringify(arg).includes('workflow-trace')
    )
    expect(traceCalls).toHaveLength(0)
  })
})
