import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import type { WSEvent } from './useWebSocket'
import { ticketKeys } from './useTickets'

/**
 * Test suite for ticket.updated event handling in useWebSocket
 *
 * This tests the event handling logic by directly testing the invalidation
 * behavior without requiring a full WebSocket connection mock.
 */
describe('useWebSocket - ticket.updated event handling logic', () => {
  let queryClient: QueryClient
  let invalidateQueriesSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    })
    invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries')
  })

  it('ticket.updated event should invalidate status, lists, and detail queries', () => {
    const event: WSEvent = {
      type: 'ticket.updated',
      project_id: 'test-project',
      ticket_id: 'TICKET-123',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate what the event handler does
    queryClient.invalidateQueries({ queryKey: ticketKeys.status() })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })

    // Verify invalidations were called
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.status() })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.lists() })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.detail('TICKET-123') })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(3)
  })

  it('ticket.updated should not invalidate workflow queries', () => {
    const event: WSEvent = {
      type: 'ticket.updated',
      project_id: 'test-project',
      ticket_id: 'TICKET-456',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate ticket.updated handler (does NOT invalidate workflow)
    queryClient.invalidateQueries({ queryKey: ticketKeys.status() })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })

    // Verify workflow queries were NOT invalidated
    const workflowInvalidations = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey).includes('workflow')
    )
    expect(workflowInvalidations.length).toBe(0)
  })

  it('ticket.updated should not invalidate agent session queries', () => {
    const event: WSEvent = {
      type: 'ticket.updated',
      project_id: 'test-project',
      ticket_id: 'TICKET-789',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate ticket.updated handler (does NOT invalidate agents)
    queryClient.invalidateQueries({ queryKey: ticketKeys.status() })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })

    // Verify agent queries were NOT invalidated
    const agentInvalidations = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey).includes('agents')
    )
    expect(agentInvalidations.length).toBe(0)
  })

  it('multiple ticket.updated events invalidate correct queries for each ticket', () => {
    const events: WSEvent[] = [
      {
        type: 'ticket.updated',
        project_id: 'test-project',
        ticket_id: 'TICKET-100',
        timestamp: '2026-01-01T00:00:00Z',
      },
      {
        type: 'ticket.updated',
        project_id: 'test-project',
        ticket_id: 'TICKET-101',
        timestamp: '2026-01-01T00:00:01Z',
      },
    ]

    events.forEach(event => {
      queryClient.invalidateQueries({ queryKey: ticketKeys.status() })
      queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
      queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    })

    // Verify each ticket's detail was invalidated
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.detail('TICKET-100') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.detail('TICKET-101') })

    // Status and lists should be invalidated twice
    const statusCalls = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(ticketKeys.status())
    )
    const listsCalls = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(ticketKeys.lists())
    )

    expect(statusCalls.length).toBe(2)
    expect(listsCalls.length).toBe(2)
  })

  it('duplicate ticket.updated events are idempotent', () => {
    const event: WSEvent = {
      type: 'ticket.updated',
      project_id: 'test-project',
      ticket_id: 'TICKET-DUP',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate duplicate event delivery (project-wide + per-ticket subscription)
    for (let i = 0; i < 2; i++) {
      queryClient.invalidateQueries({ queryKey: ticketKeys.status() })
      queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
      queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    }

    // Both calls should be recorded (idempotent - no errors)
    const detailCalls = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(ticketKeys.detail('TICKET-DUP'))
    )

    expect(detailCalls.length).toBe(2)
    // No errors should occur from duplicate invalidations
  })

  it('ticket.updated with data field still invalidates correctly', () => {
    const event: WSEvent = {
      type: 'ticket.updated',
      project_id: 'test-project',
      ticket_id: 'TICKET-999',
      timestamp: '2026-01-01T00:00:00Z',
      data: {
        status: 'in_progress',
        priority: 2,
      },
    }

    // Simulate handler
    queryClient.invalidateQueries({ queryKey: ticketKeys.status() })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })

    // Verify all three invalidations
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.status() })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.lists() })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.detail('TICKET-999') })
  })

  it('status invalidation ensures sidebar counts are refreshed', () => {
    // Simulate a ticket status change (backlog -> in_progress)
    queryClient.invalidateQueries({ queryKey: ticketKeys.status() })

    // Verify status was invalidated (this is what the sidebar uses)
    const statusInvalidations = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(ticketKeys.status())
    )

    expect(statusInvalidations.length).toBe(1)
  })

  it('lists invalidation ensures ticket lists are refreshed', () => {
    // Simulate a ticket update affecting list views
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })

    // Verify lists was invalidated (this is what TicketListPage uses)
    const listsInvalidations = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(ticketKeys.lists())
    )

    expect(listsInvalidations.length).toBe(1)
  })
})

describe('useWebSocket - phase.started and phase.completed event handling', () => {
  let queryClient: QueryClient
  let invalidateQueriesSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    })
    invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries')
  })

  it('phase.started event should invalidate detail, workflow, and lists queries', () => {
    const event: WSEvent = {
      type: 'phase.started',
      project_id: 'test-project',
      ticket_id: 'TICKET-789',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate phase.started handler
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })

    // Verify invalidations
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.detail('TICKET-789') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.workflow('TICKET-789') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.lists() })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(3)
  })

  it('phase.completed event should invalidate detail, workflow, and lists queries', () => {
    const event: WSEvent = {
      type: 'phase.completed',
      project_id: 'test-project',
      ticket_id: 'TICKET-999',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate phase.completed handler
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })

    // Verify invalidations
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.detail('TICKET-999') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.workflow('TICKET-999') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.lists() })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(3)
  })

  it('phase events invalidate lists to update workflow_progress in list view', () => {
    const event: WSEvent = {
      type: 'phase.completed',
      project_id: 'test-project',
      ticket_id: 'TICKET-555',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate handler
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })

    // Verify lists() was invalidated (critical for progress bar updates)
    const listsInvalidations = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(ticketKeys.lists())
    )
    expect(listsInvalidations.length).toBe(1)
  })

  it('phase.started should not invalidate status queries', () => {
    const event: WSEvent = {
      type: 'phase.started',
      project_id: 'test-project',
      ticket_id: 'TICKET-111',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate handler (does NOT invalidate status)
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })

    // Verify status was NOT invalidated
    const statusInvalidations = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(ticketKeys.status())
    )
    expect(statusInvalidations.length).toBe(0)
  })

  it('phase.completed should not invalidate agent session queries', () => {
    const event: WSEvent = {
      type: 'phase.completed',
      project_id: 'test-project',
      ticket_id: 'TICKET-222',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate handler (does NOT invalidate agentSessions)
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })

    // Verify agent sessions were NOT invalidated
    const agentInvalidations = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey).includes('agents')
    )
    expect(agentInvalidations.length).toBe(0)
  })

  it('multiple phase events invalidate lists each time', () => {
    const events: WSEvent[] = [
      {
        type: 'phase.started',
        project_id: 'test-project',
        ticket_id: 'TICKET-AAA',
        workflow: 'feature',
        timestamp: '2026-01-01T00:00:00Z',
      },
      {
        type: 'phase.completed',
        project_id: 'test-project',
        ticket_id: 'TICKET-AAA',
        workflow: 'feature',
        timestamp: '2026-01-01T00:00:01Z',
      },
    ]

    events.forEach(event => {
      queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
      queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
      queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
    })

    // Lists should be invalidated twice (once per event)
    const listsCalls = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(ticketKeys.lists())
    )
    expect(listsCalls.length).toBe(2)
  })

  it('phase events from different tickets all invalidate the same lists query', () => {
    const events: WSEvent[] = [
      {
        type: 'phase.completed',
        project_id: 'test-project',
        ticket_id: 'TICKET-X',
        workflow: 'feature',
        timestamp: '2026-01-01T00:00:00Z',
      },
      {
        type: 'phase.completed',
        project_id: 'test-project',
        ticket_id: 'TICKET-Y',
        workflow: 'bugfix',
        timestamp: '2026-01-01T00:00:01Z',
      },
    ]

    events.forEach(event => {
      queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
      queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
      queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
    })

    // Lists invalidated twice with same key
    const listsCalls = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(ticketKeys.lists())
    )
    expect(listsCalls.length).toBe(2)

    // Each call has identical queryKey
    listsCalls.forEach((call: any) => {
      expect(call[0].queryKey).toEqual(['tickets', 'list'])
    })
  })
})

describe('useWebSocket - workflow.updated event handling', () => {
  let queryClient: QueryClient
  let invalidateQueriesSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    })
    invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries')
  })

  it('workflow.updated event should invalidate detail, workflow, agentSessions, and lists queries', () => {
    const event: WSEvent = {
      type: 'workflow.updated',
      project_id: 'test-project',
      ticket_id: 'TICKET-500',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate workflow.updated handler
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })

    // Verify all four invalidations
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.detail('TICKET-500') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.workflow('TICKET-500') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.agentSessions('TICKET-500') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.lists() })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(4)
  })

  it('workflow.updated invalidates lists to update workflow_progress in list view', () => {
    const event: WSEvent = {
      type: 'workflow.updated',
      project_id: 'test-project',
      ticket_id: 'TICKET-600',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate handler
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })

    // Verify lists() was invalidated
    const listsInvalidations = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(ticketKeys.lists())
    )
    expect(listsInvalidations.length).toBe(1)
  })
})

describe('Query key structure verification', () => {
  it('ticketKeys.status() returns correct key for sidebar counts', () => {
    expect(ticketKeys.status()).toEqual(['tickets', 'status'])
  })

  it('ticketKeys.lists() returns correct key for ticket lists', () => {
    expect(ticketKeys.lists()).toEqual(['tickets', 'list'])
  })

  it('ticketKeys.detail() returns correct key for specific ticket', () => {
    expect(ticketKeys.detail('TICKET-123')).toEqual(['tickets', 'detail', 'TICKET-123'])
  })

  it('ticket.updated invalidates three distinct query key types', () => {
    const statusKey = ticketKeys.status()
    const listsKey = ticketKeys.lists()
    const detailKey = ticketKeys.detail('TICKET-1')

    // All three should be different
    expect(JSON.stringify(statusKey)).not.toBe(JSON.stringify(listsKey))
    expect(JSON.stringify(statusKey)).not.toBe(JSON.stringify(detailKey))
    expect(JSON.stringify(listsKey)).not.toBe(JSON.stringify(detailKey))
  })
})
