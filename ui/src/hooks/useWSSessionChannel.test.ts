import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { handleSessionScopedEvent, deniedSessionID } from './useWSSessionChannel'
import { sessionKeys } from './useSessions'
import { sessionFlowKeys } from './useSessionFlow'
import type { WSEventV2 } from './useWSProtocol'

function makeEvent(type: string, overrides: Partial<WSEventV2> = {}): WSEventV2 {
  return {
    type,
    project_id: 'proj1',
    ticket_id: '',
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
})

function hasKey(key: unknown) {
  const serialised = JSON.stringify(key)
  return spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === serialised)
}

describe('handleSessionScopedEvent', () => {
  it('invalidates session-messages for a session-scoped messages.updated event', () => {
    handleSessionScopedEvent(makeEvent('messages.updated', { session_id: 'sid-1' }), queryClient)
    expect(hasKey(['session-messages', 'sid-1'])).toBe(true)
  })

  it('does nothing for messages.updated without a session_id', () => {
    handleSessionScopedEvent(makeEvent('messages.updated'), queryClient)
    expect(spy).not.toHaveBeenCalled()
  })

  it.each(['session.cost_updated', 'console_chat.sibling_opened'])(
    'invalidates sessionKeys.all and the per-session flow/stats keys for %s',
    (type) => {
      handleSessionScopedEvent(makeEvent(type, { session_id: 'sid-2' }), queryClient)
      expect(hasKey(sessionKeys.all)).toBe(true)
      expect(hasKey(sessionFlowKeys.flow('sid-2'))).toBe(true)
      expect(hasKey(sessionFlowKeys.stats('sid-2'))).toBe(true)
    }
  )

  it('invalidates sessionKeys.all but not per-session keys when session_id is absent', () => {
    handleSessionScopedEvent(makeEvent('session.cost_updated'), queryClient)
    expect(hasKey(sessionKeys.all)).toBe(true)
    expect(spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey).startsWith('["session-flow"'))).toBe(
      false
    )
  })

  it('ignores unrelated event types', () => {
    handleSessionScopedEvent(makeEvent('agent.started', { session_id: 'sid-3' }), queryClient)
    expect(spy).not.toHaveBeenCalled()
  })
})

describe('deniedSessionID', () => {
  it('extracts the session id from a session_subscription_denied ack', () => {
    expect(deniedSessionID({ action: 'session_subscription_denied', session_id: 'sid-4' })).toBe('sid-4')
  })

  it('returns null for any other ack action', () => {
    expect(deniedSessionID({ action: 'subscribed', session_id: 'sid-4' })).toBeNull()
  })

  it('returns null when session_id is missing', () => {
    expect(deniedSessionID({ action: 'session_subscription_denied' })).toBeNull()
  })
})
