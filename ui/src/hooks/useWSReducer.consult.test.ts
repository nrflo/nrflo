import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import { ticketKeys, projectWorkflowKeys } from './useTickets'
import type { WSEventV2 } from './useWSProtocol'

function makeEvent(type: string, overrides: Partial<WSEventV2> = {}): WSEventV2 {
  return {
    type,
    project_id: 'proj1',
    ticket_id: 'TICK-1',
    timestamp: '2026-01-01T00:00:00Z',
    sequence: 1,
    protocol_version: 2,
    ...overrides,
  }
}

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

function hasKey(key: unknown) {
  const serialised = JSON.stringify(key)
  return spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === serialised)
}

describe('useWSReducer - consult.started', () => {
  it('invalidates ticket workflow for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('consult.started'), queryClient)
    expect(hasKey(ticketKeys.workflow('TICK-1'))).toBe(true)
  })

  it('invalidates ticket detail for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('consult.started'), queryClient)
    expect(hasKey(ticketKeys.detail('TICK-1'))).toBe(true)
  })

  it('invalidates session-messages prefix query', () => {
    dispatchV2Event(makeEvent('consult.started'), queryClient)
    expect(hasKey(['session-messages'])).toBe(true)
  })

  it('invalidates project workflow for project-scoped event', () => {
    dispatchV2Event(makeEvent('consult.started', { ticket_id: '', sequence: 2 }), queryClient)
    expect(hasKey(projectWorkflowKeys.workflow('proj1'))).toBe(true)
  })

  it('deduplicates by sequence', () => {
    const event = makeEvent('consult.started', { sequence: 10 })
    dispatchV2Event(event, queryClient)
    dispatchV2Event(event, queryClient)
    const calls = spy.mock.calls.filter((call: any) =>
      JSON.stringify(call[0].queryKey) === JSON.stringify(ticketKeys.workflow('TICK-1'))
    )
    expect(calls).toHaveLength(1)
  })
})

describe('useWSReducer - consult.answered', () => {
  it('invalidates ticket workflow for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('consult.answered', { sequence: 20 }), queryClient)
    expect(hasKey(ticketKeys.workflow('TICK-1'))).toBe(true)
  })

  it('invalidates session-messages prefix query', () => {
    dispatchV2Event(makeEvent('consult.answered', { sequence: 21 }), queryClient)
    expect(hasKey(['session-messages'])).toBe(true)
  })

  it('invalidates project workflow for project-scoped event', () => {
    dispatchV2Event(makeEvent('consult.answered', { ticket_id: '', sequence: 22 }), queryClient)
    expect(hasKey(projectWorkflowKeys.workflow('proj1'))).toBe(true)
  })
})

describe('useWSReducer - consult.failed', () => {
  it('invalidates ticket workflow for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('consult.failed', { sequence: 30 }), queryClient)
    expect(hasKey(ticketKeys.workflow('TICK-1'))).toBe(true)
  })

  it('invalidates session-messages prefix query', () => {
    dispatchV2Event(makeEvent('consult.failed', { sequence: 31 }), queryClient)
    expect(hasKey(['session-messages'])).toBe(true)
  })

  it('does not invalidate session-messages for project-scoped event (still fires)', () => {
    dispatchV2Event(makeEvent('consult.failed', { ticket_id: '', sequence: 32 }), queryClient)
    // project scope: no ticket keys, but session-messages still invalidated
    expect(hasKey(['session-messages'])).toBe(true)
    expect(hasKey(projectWorkflowKeys.workflow('proj1'))).toBe(true)
  })
})
