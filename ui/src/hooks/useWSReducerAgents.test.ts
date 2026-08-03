import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import { ticketKeys, projectWorkflowKeys } from './useTickets'
import { traceKeys } from './useTrace'
import { systemAgentRunKeys } from './useSystemAgentRuns'
import { sessionKeys } from './useSessions'
import { sessionFlowKeys } from './useSessionFlow'
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

describe.each(['delegate.started', 'delegate.completed', 'delegate.failed'])('useWSReducerAgents - %s', (type) => {
  it('invalidates the agent-session set, trace, and system-agent-runs keys for a ticket-scoped event', () => {
    dispatchV2Event(makeEvent(type, { sequence: 100 }), queryClient)
    expect(hasKey(ticketKeys.workflow('TICK-1'))).toBe(true)
    expect(hasKey(ticketKeys.detail('TICK-1'))).toBe(true)
    expect(hasKey(ticketKeys.agentSessions('TICK-1'))).toBe(true)
    expect(hasKey(traceKeys.all)).toBe(true)
    expect(hasKey(systemAgentRunKeys.all)).toBe(true)
  })

  it('invalidates the project-scoped agent set for a project-scoped event', () => {
    dispatchV2Event(makeEvent(type, { ticket_id: '', sequence: 101 }), queryClient)
    expect(hasKey(projectWorkflowKeys.workflow('proj1'))).toBe(true)
    expect(hasKey(projectWorkflowKeys.agentSessions('proj1'))).toBe(true)
    expect(hasKey(traceKeys.all)).toBe(true)
    expect(hasKey(systemAgentRunKeys.all)).toBe(true)
  })

  it('does not invalidate session-messages (that is consult-only behaviour)', () => {
    dispatchV2Event(makeEvent(type, { sequence: 102 }), queryClient)
    expect(hasKey(['session-messages'])).toBe(false)
  })

  it('invalidates sessionKeys.all, and the specific session flow/stats keys when session_id is present', () => {
    dispatchV2Event(makeEvent(type, { sequence: 103, session_id: 'sid-1' }), queryClient)
    expect(hasKey(sessionKeys.all)).toBe(true)
    expect(hasKey(sessionFlowKeys.flow('sid-1'))).toBe(true)
    expect(hasKey(sessionFlowKeys.stats('sid-1'))).toBe(true)
  })

  it('invalidates sessionKeys.all but no per-session flow/stats keys without session_id', () => {
    dispatchV2Event(makeEvent(type, { sequence: 104 }), queryClient)
    expect(hasKey(sessionKeys.all)).toBe(true)
    expect(spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey).startsWith('["session-flow"'))).toBe(
      false
    )
  })
})

describe('useWSReducerAgents - consult.* unaffected by the delegate move', () => {
  it('still invalidates session-messages but not trace/system-agent-runs keys', () => {
    dispatchV2Event(makeEvent('consult.started', { sequence: 200 }), queryClient)
    expect(hasKey(['session-messages'])).toBe(true)
    expect(hasKey(traceKeys.all)).toBe(false)
    expect(hasKey(systemAgentRunKeys.all)).toBe(false)
  })

  it('invalidates sessionKeys.all and the session-scoped flow/stats keys when session_id is present', () => {
    dispatchV2Event(makeEvent('consult.started', { sequence: 201, session_id: 'sid-2' }), queryClient)
    expect(hasKey(sessionKeys.all)).toBe(true)
    expect(hasKey(sessionFlowKeys.flow('sid-2'))).toBe(true)
    expect(hasKey(sessionFlowKeys.stats('sid-2'))).toBe(true)
  })
})
