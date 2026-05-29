import { describe, it, expect, beforeEach, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import type { WSEventV2 } from './useWSProtocol'

describe('useWSReducer - ticket.updated', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    clearSeqs()
    sessionStorage.clear()
    vi.spyOn(queryClient, 'invalidateQueries')
  })

  const makeEvent = (overrides: Partial<WSEventV2> = {}): WSEventV2 => ({
    type: 'ticket.updated',
    project_id: 'proj1',
    ticket_id: 'tick-updated-1',
    timestamp: '2026-02-15T00:00:00Z',
    sequence: 100,
    protocol_version: 2,
    ...overrides,
  })

  it('invalidates ticketKeys.status()', () => {
    dispatchV2Event(makeEvent(), queryClient)

    const calls = (queryClient.invalidateQueries as any).mock.calls
    expect(calls.some((call: any) =>
      JSON.stringify(call[0].queryKey).includes('status')
    )).toBe(true)
  })

  it('invalidates ticketKeys.lists()', () => {
    dispatchV2Event(makeEvent({ sequence: 101 }), queryClient)

    const calls = (queryClient.invalidateQueries as any).mock.calls
    expect(calls.some((call: any) =>
      JSON.stringify(call[0].queryKey).includes('list')
    )).toBe(true)
  })

  it('invalidates ticketKeys.detail(ticket_id)', () => {
    dispatchV2Event(makeEvent({ sequence: 102 }), queryClient)

    const calls = (queryClient.invalidateQueries as any).mock.calls
    expect(calls.some((call: any) =>
      JSON.stringify(call[0].queryKey).includes('tick-updated-1')
    )).toBe(true)
  })

  it('invalidates dailyStatsKeys.all', () => {
    dispatchV2Event(makeEvent({ sequence: 103 }), queryClient)

    const calls = (queryClient.invalidateQueries as any).mock.calls
    expect(calls.some((call: any) =>
      JSON.stringify(call[0].queryKey).includes('daily-stats')
    )).toBe(true)
  })

  it('all four keys invalidated in a single dispatch', () => {
    dispatchV2Event(makeEvent({ sequence: 104 }), queryClient)

    const calls = (queryClient.invalidateQueries as any).mock.calls
    const keys = calls.map((call: any) => JSON.stringify(call[0].queryKey))

    expect(keys.some((k: string) => k.includes('status'))).toBe(true)
    expect(keys.some((k: string) => k.includes('list'))).toBe(true)
    expect(keys.some((k: string) => k.includes('detail'))).toBe(true)
    expect(keys.some((k: string) => k.includes('daily-stats'))).toBe(true)
  })

  it('duplicate event with same sequence is not re-processed (idempotency)', () => {
    const event = makeEvent({ sequence: 105 })
    dispatchV2Event(event, queryClient)
    const firstCallCount = (queryClient.invalidateQueries as any).mock.calls.length

    // Replay same event — seq already seen, should be dropped
    dispatchV2Event(event, queryClient)
    const secondCallCount = (queryClient.invalidateQueries as any).mock.calls.length

    expect(secondCallCount).toBe(firstCallCount)
  })
})
