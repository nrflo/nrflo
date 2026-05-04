import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import { workflowChainRunKeys } from './useWorkflowChains'
import type { WSEventV2 } from './useWSProtocol'

const CHAIN_RUN_EVENTS = [
  'chain.run_started',
  'chain.step_started',
  'chain.step_completed',
  'chain.run_completed',
  'chain.run_failed',
] as const

function makeEvent(type: string, seq = 1): WSEventV2 {
  return {
    type,
    project_id: 'proj1',
    ticket_id: '',
    timestamp: '2026-01-01T00:00:00Z',
    sequence: seq,
    protocol_version: 2,
  }
}

describe('useWSReducer - chain run events', () => {
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

  CHAIN_RUN_EVENTS.forEach((eventType, idx) => {
    it(`${eventType} invalidates workflowChainRunKeys.all`, () => {
      dispatchV2Event(makeEvent(eventType, idx + 1), queryClient)

      const expectedKey = JSON.stringify(workflowChainRunKeys.all)
      expect(
        spy.mock.calls.some((call: any) =>
          JSON.stringify(call[0].queryKey) === expectedKey
        )
      ).toBe(true)
    })
  })

  it('duplicate sequence for chain.run_started is idempotent', () => {
    const event = makeEvent('chain.run_started', 42)
    dispatchV2Event(event, queryClient)
    dispatchV2Event(event, queryClient) // same seq — ignored

    const expectedKey = JSON.stringify(workflowChainRunKeys.all)
    const matchingCalls = spy.mock.calls.filter((call: any) =>
      JSON.stringify(call[0].queryKey) === expectedKey
    )
    expect(matchingCalls).toHaveLength(1)
  })

  it('all five chain run event types each trigger an invalidation', () => {
    CHAIN_RUN_EVENTS.forEach((type, idx) => {
      dispatchV2Event(makeEvent(type, idx + 100), queryClient)
    })

    const expectedKey = JSON.stringify(workflowChainRunKeys.all)
    const matchingCalls = spy.mock.calls.filter((call: any) =>
      JSON.stringify(call[0].queryKey) === expectedKey
    )
    expect(matchingCalls.length).toBe(CHAIN_RUN_EVENTS.length)
  })
})
