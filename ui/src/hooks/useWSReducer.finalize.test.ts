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

describe('useWSReducer - workflow.finalize_succeeded', () => {
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
    dispatchV2Event(makeEvent('workflow.finalize_succeeded'), queryClient)

    const workflowKey = JSON.stringify(ticketKeys.workflow('TICK-1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === workflowKey)
    ).toBe(true)
  })

  it('invalidates ticket detail for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('workflow.finalize_succeeded'), queryClient)

    const detailKey = JSON.stringify(ticketKeys.detail('TICK-1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === detailKey)
    ).toBe(true)
  })

  it('invalidates ticket lists for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('workflow.finalize_succeeded'), queryClient)

    const listsKey = JSON.stringify(ticketKeys.lists())
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === listsKey)
    ).toBe(true)
  })

  it('invalidates project workflow query for project-scoped event (empty ticket_id)', () => {
    dispatchV2Event(
      makeEvent('workflow.finalize_succeeded', { ticket_id: '', sequence: 2 }),
      queryClient
    )

    const projKey = JSON.stringify(projectWorkflowKeys.workflow('proj1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === projKey)
    ).toBe(true)
  })

  it('only invalidates once per unique sequence', () => {
    const event = makeEvent('workflow.finalize_succeeded', { sequence: 7 })
    dispatchV2Event(event, queryClient)
    dispatchV2Event(event, queryClient) // duplicate

    const workflowKey = JSON.stringify(ticketKeys.workflow('TICK-1'))
    const calls = spy.mock.calls.filter((call: any) =>
      JSON.stringify(call[0].queryKey) === workflowKey
    )
    expect(calls).toHaveLength(1)
  })
})

describe('useWSReducer - workflow.finalize_failed', () => {
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
    dispatchV2Event(makeEvent('workflow.finalize_failed', { sequence: 10 }), queryClient)

    const workflowKey = JSON.stringify(ticketKeys.workflow('TICK-1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === workflowKey)
    ).toBe(true)
  })

  it('invalidates ticket detail for ticket-scoped event', () => {
    dispatchV2Event(makeEvent('workflow.finalize_failed', { sequence: 11 }), queryClient)

    const detailKey = JSON.stringify(ticketKeys.detail('TICK-1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === detailKey)
    ).toBe(true)
  })

  it('invalidates project workflow query for project-scoped event', () => {
    dispatchV2Event(
      makeEvent('workflow.finalize_failed', { ticket_id: '', sequence: 12 }),
      queryClient
    )

    const projKey = JSON.stringify(projectWorkflowKeys.workflow('proj1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === projKey)
    ).toBe(true)
  })
})
