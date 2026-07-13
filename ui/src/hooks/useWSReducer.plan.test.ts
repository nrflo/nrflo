import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import { ticketKeys, projectWorkflowKeys } from './useTickets'
import { planKeys } from './usePlan'
import type { WSEventV2 } from './useWSProtocol'

const PLAN_EVENT_TYPES = [
  'plan.drafted',
  'plan.revised',
  'plan.approved',
  'plan.cancelled',
  'plan.materialized',
  'workflow.plan_waiting',
] as const

function makeEvent(type: string, overrides: Partial<WSEventV2> = {}): WSEventV2 {
  return {
    type,
    project_id: 'proj1',
    ticket_id: 'TICK-1',
    timestamp: '2026-01-01T00:00:00Z',
    sequence: 1,
    protocol_version: 2,
    data: { instance_id: 'inst-1' },
    ...overrides,
  }
}

describe.each(PLAN_EVENT_TYPES)('useWSReducer - %s', (eventType) => {
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

  function keyWasInvalidated(key: readonly unknown[]): boolean {
    const target = JSON.stringify(key)
    return spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === target)
  }

  it('invalidates the plan detail query using data.instance_id', () => {
    dispatchV2Event(makeEvent(eventType, { sequence: 10 }), queryClient)
    expect(keyWasInvalidated(planKeys.detail('inst-1'))).toBe(true)
  })

  it('invalidates ticket workflow + detail queries for ticket-scoped event', () => {
    dispatchV2Event(makeEvent(eventType, { sequence: 11 }), queryClient)
    expect(keyWasInvalidated(ticketKeys.workflow('TICK-1'))).toBe(true)
    expect(keyWasInvalidated(ticketKeys.detail('TICK-1'))).toBe(true)
  })

  it('invalidates project workflow query for project-scoped event (empty ticket_id)', () => {
    dispatchV2Event(
      makeEvent(eventType, { ticket_id: '', sequence: 12 }),
      queryClient
    )
    expect(keyWasInvalidated(projectWorkflowKeys.workflow('proj1'))).toBe(true)
  })

  it('does not invalidate ticket keys for project-scoped event', () => {
    dispatchV2Event(
      makeEvent(eventType, { ticket_id: '', sequence: 13 }),
      queryClient
    )
    expect(keyWasInvalidated(ticketKeys.detail('TICK-1'))).toBe(false)
  })

  it('skips the plan detail invalidation when data.instance_id is missing', () => {
    dispatchV2Event(makeEvent(eventType, { sequence: 14, data: {} }), queryClient)
    expect(keyWasInvalidated(planKeys.detail('inst-1'))).toBe(false)
  })

  it('is sequence-idempotent — duplicate seq does not re-invalidate', () => {
    const event = makeEvent(eventType, { sequence: 15 })
    dispatchV2Event(event, queryClient)
    const handled = dispatchV2Event(event, queryClient)
    expect(handled).toBe(false)

    const calls = spy.mock.calls.filter(
      (call: any) => JSON.stringify(call[0].queryKey) === JSON.stringify(planKeys.detail('inst-1'))
    )
    expect(calls).toHaveLength(1)
  })
})
