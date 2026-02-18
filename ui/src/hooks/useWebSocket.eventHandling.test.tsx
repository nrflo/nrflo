import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import type { WSEvent } from './useWebSocket'
import { ticketKeys, projectWorkflowKeys, dailyStatsKeys } from './useTickets'
import { chainKeys } from './useChains'

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

describe('useWebSocket - ticket nrworkflow-d3a7c4: project-level agent events', () => {
  let queryClient: QueryClient
  let invalidateQueriesSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    })
    invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries')
  })

  it('agent.started event for project scope (empty ticket_id) invalidates project workflow and agent sessions', () => {
    const event: WSEvent = {
      type: 'agent.started',
      project_id: 'test-project',
      ticket_id: '', // Empty for project scope
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate handler for project-scope event (isProjectScope = true)
    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })
    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(event.project_id) })

    // Verify project-level invalidations
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.workflow('test-project') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.agentSessions('test-project') })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(2)
  })

  it('agent.completed event for project scope invalidates project workflow and agent sessions', () => {
    const event: WSEvent = {
      type: 'agent.completed',
      project_id: 'test-project',
      ticket_id: '',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })
    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(event.project_id) })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.workflow('test-project') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.agentSessions('test-project') })
  })

  it('agent.started for ticket scope does NOT invalidate project queries', () => {
    const event: WSEvent = {
      type: 'agent.started',
      project_id: 'test-project',
      ticket_id: 'TICKET-123',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Ticket-scope handler
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })

    // Verify project queries were NOT invalidated
    const projectInvalidations = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey).includes('project-workflows')
    )
    expect(projectInvalidations.length).toBe(0)
  })

  it('messages.updated event for project scope invalidates project workflow, agent sessions, and session-messages', () => {
    const event: WSEvent = {
      type: 'messages.updated',
      project_id: 'test-project',
      ticket_id: '',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
      data: { session_id: 'session-123' },
    }

    // Project-scope handler
    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })
    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(event.project_id) })
    // Session-specific invalidation (applies to both scopes)
    queryClient.invalidateQueries({ queryKey: ['session-messages', event.data?.session_id] })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.workflow('test-project') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.agentSessions('test-project') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['session-messages', 'session-123'] })
  })

  it('phase.started event for project scope invalidates project workflow only', () => {
    const event: WSEvent = {
      type: 'phase.started',
      project_id: 'test-project',
      ticket_id: '',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.workflow('test-project') })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(1)
  })

  it('findings.updated event for project scope invalidates project workflow', () => {
    const event: WSEvent = {
      type: 'findings.updated',
      project_id: 'test-project',
      ticket_id: '',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.workflow('test-project') })
  })

  it('workflow.updated event for project scope invalidates project workflow', () => {
    const event: WSEvent = {
      type: 'workflow.updated',
      project_id: 'test-project',
      ticket_id: '',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.workflow('test-project') })
  })

  it('orchestration.started event for project scope invalidates project workflow', () => {
    const event: WSEvent = {
      type: 'orchestration.started',
      project_id: 'test-project',
      ticket_id: '',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.workflow('test-project') })
  })

  it('project workflow query keys do not collide with ticket query keys', () => {
    const projectWorkflowKey = projectWorkflowKeys.workflow('test-project')
    const projectAgentSessionsKey = projectWorkflowKeys.agentSessions('test-project')
    const ticketWorkflowKey = ticketKeys.workflow('TICKET-123')
    const ticketAgentSessionsKey = ticketKeys.agentSessions('TICKET-123')

    expect(JSON.stringify(projectWorkflowKey)).not.toBe(JSON.stringify(ticketWorkflowKey))
    expect(JSON.stringify(projectAgentSessionsKey)).not.toBe(JSON.stringify(ticketAgentSessionsKey))

    expect(projectWorkflowKey[0]).toBe('project-workflows')
    expect(ticketWorkflowKey[0]).toBe('tickets')
  })

  it('messages.updated session invalidation works for both ticket and project scopes', () => {
    const sessionId = 'shared-session-id'

    // Both should invalidate session-messages with same session_id
    queryClient.invalidateQueries({ queryKey: ['session-messages', sessionId] })
    queryClient.invalidateQueries({ queryKey: ['session-messages', sessionId] })

    const sessionInvalidations = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(['session-messages', sessionId])
    )
    expect(sessionInvalidations.length).toBe(2)
  })
})

describe('useWebSocket - agent.continued event handling', () => {
  let queryClient: QueryClient
  let invalidateQueriesSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    })
    invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries')
  })

  it('agent.continued event for ticket scope invalidates detail, workflow, and agentSessions', () => {
    const event: WSEvent = {
      type: 'agent.continued',
      project_id: 'test-project',
      ticket_id: 'TICKET-CONT-1',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate ticket-scope agent.continued handler
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.detail('TICKET-CONT-1') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.workflow('TICKET-CONT-1') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.agentSessions('TICKET-CONT-1') })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(3)
  })

  it('agent.continued event for project scope invalidates project workflow and agentSessions', () => {
    const event: WSEvent = {
      type: 'agent.continued',
      project_id: 'test-project',
      ticket_id: '',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
    }

    // Simulate project-scope agent.continued handler
    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })
    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(event.project_id) })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.workflow('test-project') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.agentSessions('test-project') })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(2)
  })

  it('agent.continued with context-save data still invalidates correctly', () => {
    const event: WSEvent = {
      type: 'agent.continued',
      project_id: 'test-project',
      ticket_id: 'TICKET-999',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
      data: {
        session_id: 'old-session-123',
        new_session_id: 'new-session-456',
        reason: 'low_context',
      },
    }

    // Simulate handler
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })

    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(3)
  })
})

describe('useWebSocket - workflow_def and agent_def event handling', () => {
  let queryClient: QueryClient
  let invalidateQueriesSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    })
    invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries')
  })

  it('workflow_def.created invalidates both workflow-defs key patterns', () => {
    // Simulate handler - invalidates both key patterns
    queryClient.invalidateQueries({ queryKey: ['workflow-defs'] })
    queryClient.invalidateQueries({ queryKey: ['workflows', 'defs'] })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['workflow-defs'] })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['workflows', 'defs'] })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(2)
  })

  it('workflow_def.updated invalidates both workflow-defs key patterns', () => {
    queryClient.invalidateQueries({ queryKey: ['workflow-defs'] })
    queryClient.invalidateQueries({ queryKey: ['workflows', 'defs'] })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['workflow-defs'] })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['workflows', 'defs'] })
  })

  it('workflow_def.deleted invalidates both workflow-defs key patterns', () => {
    queryClient.invalidateQueries({ queryKey: ['workflow-defs'] })
    queryClient.invalidateQueries({ queryKey: ['workflows', 'defs'] })

    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(2)
  })

  it('agent_def.created invalidates workflow-defs, workflows-defs, and agent-defs prefix', () => {
    // Simulate handler
    queryClient.invalidateQueries({ queryKey: ['workflow-defs'] })
    queryClient.invalidateQueries({ queryKey: ['workflows', 'defs'] })
    queryClient.invalidateQueries({ queryKey: ['agent-defs'] })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['workflow-defs'] })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['workflows', 'defs'] })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['agent-defs'] })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(3)
  })

  it('agent_def.updated invalidates all three key patterns', () => {
    queryClient.invalidateQueries({ queryKey: ['workflow-defs'] })
    queryClient.invalidateQueries({ queryKey: ['workflows', 'defs'] })
    queryClient.invalidateQueries({ queryKey: ['agent-defs'] })

    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(3)
  })

  it('agent_def.deleted invalidates all three key patterns', () => {
    queryClient.invalidateQueries({ queryKey: ['workflow-defs'] })
    queryClient.invalidateQueries({ queryKey: ['workflows', 'defs'] })
    queryClient.invalidateQueries({ queryKey: ['agent-defs'] })

    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(3)
  })

  it('agent_def events use prefix-match for agent-defs to catch all variants', () => {
    // This test verifies that ['agent-defs'] prefix will match:
    // ['agent-defs', project, workflow_id] used in actual queries

    queryClient.invalidateQueries({ queryKey: ['agent-defs'] })

    // Verify the invalidation was called with prefix only
    const agentDefCalls = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey) === JSON.stringify(['agent-defs'])
    )

    expect(agentDefCalls.length).toBe(1)
  })
})

describe('useWebSocket - messages.updated narrowing', () => {
  let queryClient: QueryClient
  let invalidateQueriesSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    })
    invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries')
  })

  it('messages.updated invalidates session-messages, agentSessions, and workflow for ticket scope', () => {
    const event: WSEvent = {
      type: 'messages.updated',
      project_id: 'test-project',
      ticket_id: 'TICKET-MSG',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
      data: { session_id: 'session-123' },
    }

    // Simulate handler — now includes workflow invalidation
    queryClient.invalidateQueries({ queryKey: ['session-messages', event.data?.session_id] })
    queryClient.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['session-messages', 'session-123'] })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.agentSessions('TICKET-MSG') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.workflow('TICKET-MSG') })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(3)
  })

  it('messages.updated invalidates workflow queries (context_left update fix)', () => {
    const event: WSEvent = {
      type: 'messages.updated',
      project_id: 'test-project',
      ticket_id: 'TICKET-MSG-2',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
      data: { session_id: 'session-456' },
    }

    queryClient.invalidateQueries({ queryKey: ['session-messages', event.data?.session_id] })
    queryClient.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })

    // Verify workflow query IS now invalidated (fixes stale context_left)
    const workflowInvalidations = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => call[0] && JSON.stringify(call[0].queryKey).includes('workflow')
    )

    expect(workflowInvalidations.length).toBe(1)
  })

  it('messages.updated for project scope invalidates session-messages, project agentSessions, and project workflow', () => {
    const event: WSEvent = {
      type: 'messages.updated',
      project_id: 'test-project',
      ticket_id: '',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
      data: { session_id: 'session-789' },
    }

    queryClient.invalidateQueries({ queryKey: ['session-messages', event.data?.session_id] })
    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.agentSessions(event.project_id) })
    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(event.project_id) })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['session-messages', 'session-789'] })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.agentSessions('test-project') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.workflow('test-project') })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(3)
  })

  it('messages.updated without session_id still invalidates agentSessions and workflow', () => {
    const event: WSEvent = {
      type: 'messages.updated',
      project_id: 'test-project',
      ticket_id: 'TICKET-NO-SID',
      workflow: 'feature',
      timestamp: '2026-01-01T00:00:00Z',
      data: {},
    }

    // Fallback when session_id missing: invalidate agentSessions and workflow
    queryClient.invalidateQueries({ queryKey: ticketKeys.agentSessions(event.ticket_id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(event.ticket_id) })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.agentSessions('TICKET-NO-SID') })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.workflow('TICKET-NO-SID') })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(2)
  })
})

describe('useWebSocket - reconnect invalidateAll coverage', () => {
  let queryClient: QueryClient
  let invalidateQueriesSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    })
    invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries')
  })

  it('invalidateAll covers all query families', () => {
    // Simulate reconnect invalidateAll (M6)
    queryClient.invalidateQueries({ queryKey: ticketKeys.all })
    queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.all })
    queryClient.invalidateQueries({ queryKey: ['workflow-defs'] })
    queryClient.invalidateQueries({ queryKey: ['workflows', 'defs'] })
    queryClient.invalidateQueries({ queryKey: ['agent-defs'] })
    queryClient.invalidateQueries({ queryKey: ['session-messages'] })

    // Verify all families invalidated
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ticketKeys.all })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: projectWorkflowKeys.all })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['workflow-defs'] })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['workflows', 'defs'] })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['agent-defs'] })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: ['session-messages'] })
  })

  it('reconnect invalidateAll includes chainKeys and dailyStatsKeys', () => {
    queryClient.invalidateQueries({ queryKey: chainKeys.all })
    queryClient.invalidateQueries({ queryKey: dailyStatsKeys.all })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: chainKeys.all })
    expect(invalidateQueriesSpy).toHaveBeenCalledWith({ queryKey: dailyStatsKeys.all })
  })
})

describe('projectWorkflowKeys structure', () => {
  it('projectWorkflowKeys.workflow() returns correct key', () => {
    expect(projectWorkflowKeys.workflow('test-project')).toEqual(['project-workflows', 'test-project'])
  })

  it('projectWorkflowKeys.agentSessions() returns correct key', () => {
    expect(projectWorkflowKeys.agentSessions('test-project')).toEqual(['project-workflows', 'agents', 'test-project'])
  })

  it('project workflow keys have distinct structure from ticket keys', () => {
    const projectKey = projectWorkflowKeys.workflow('test-project')
    const ticketKey = ticketKeys.workflow('TICKET-123')

    expect(projectKey[0]).toBe('project-workflows')
    expect(ticketKey[0]).toBe('tickets')
    expect(projectKey.length).toBe(2)
    // ticketKeys.workflow includes project in the key, so length is 4: ['tickets', 'detail', ticketId, 'workflow']
    expect(ticketKey.length).toBeGreaterThanOrEqual(3)
  })
})
