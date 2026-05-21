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

describe('useWSReducer - workflow.paused', () => {
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

  it('invalidates ticket workflow query for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('workflow.paused', { sequence: 20 }), queryClient)

    const workflowKey = JSON.stringify(ticketKeys.workflow('TICK-1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === workflowKey)
    ).toBe(true)
  })

  it('invalidates ticket detail for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('workflow.paused', { sequence: 21 }), queryClient)

    const detailKey = JSON.stringify(ticketKeys.detail('TICK-1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === detailKey)
    ).toBe(true)
  })

  it('invalidates ticket agent sessions for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('workflow.paused', { sequence: 22 }), queryClient)

    const sessionsKey = JSON.stringify(ticketKeys.agentSessions('TICK-1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === sessionsKey)
    ).toBe(true)
  })

  it('invalidates ticket lists for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('workflow.paused', { sequence: 23 }), queryClient)

    const listsKey = JSON.stringify(ticketKeys.lists())
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === listsKey)
    ).toBe(true)
  })

  it('invalidates project workflow query for project-scoped event (empty ticket_id)', () => {
    dispatchV2Event(
      makeEvent('workflow.paused', { ticket_id: '', sequence: 24 }),
      queryClient
    )

    const projKey = JSON.stringify(projectWorkflowKeys.workflow('proj1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === projKey)
    ).toBe(true)
  })

  it('does not invalidate ticket keys for project-scoped event', () => {
    dispatchV2Event(
      makeEvent('workflow.paused', { ticket_id: '', sequence: 25 }),
      queryClient
    )

    const detailKey = JSON.stringify(ticketKeys.detail('TICK-1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === detailKey)
    ).toBe(false)
  })

  it('only invalidates once per unique sequence (idempotent)', () => {
    const event = makeEvent('workflow.paused', { sequence: 26 })
    dispatchV2Event(event, queryClient)
    dispatchV2Event(event, queryClient) // duplicate

    const workflowKey = JSON.stringify(ticketKeys.workflow('TICK-1'))
    const calls = spy.mock.calls.filter((call: any) =>
      JSON.stringify(call[0].queryKey) === workflowKey
    )
    expect(calls).toHaveLength(1)
  })
})

describe('useWSReducer - workflow.resumed', () => {
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

  it('invalidates ticket workflow query for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('workflow.resumed', { sequence: 30 }), queryClient)

    const workflowKey = JSON.stringify(ticketKeys.workflow('TICK-1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === workflowKey)
    ).toBe(true)
  })

  it('invalidates ticket detail for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('workflow.resumed', { sequence: 31 }), queryClient)

    const detailKey = JSON.stringify(ticketKeys.detail('TICK-1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === detailKey)
    ).toBe(true)
  })

  it('invalidates ticket lists for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('workflow.resumed', { sequence: 32 }), queryClient)

    const listsKey = JSON.stringify(ticketKeys.lists())
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === listsKey)
    ).toBe(true)
  })

  it('invalidates project workflow query for project-scoped event', () => {
    dispatchV2Event(
      makeEvent('workflow.resumed', { ticket_id: '', sequence: 33 }),
      queryClient
    )

    const projKey = JSON.stringify(projectWorkflowKeys.workflow('proj1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === projKey)
    ).toBe(true)
  })
})
