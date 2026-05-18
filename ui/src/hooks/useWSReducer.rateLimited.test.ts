import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { dispatchV2Event, clearSeqs } from './useWSReducer'
import { agentSessionLogKeys } from './useAgentSessionLogs'
import type { WSEventV2 } from './useWSProtocol'
import type { LiveAgentSession, LiveAgentSessionsResponse } from '@/types/agentSessionLogs'

const BASE_TIME = new Date('2026-01-01T00:00:00.000Z')

describe('useWSReducer — agent.rate_limited handler', () => {
  let qc: QueryClient

  beforeEach(() => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    clearSeqs()
    vi.useFakeTimers()
    vi.setSystemTime(BASE_TIME)
  })

  afterEach(() => {
    clearSeqs()
    vi.useRealTimers()
  })

  function makeSession(overrides: Partial<LiveAgentSession> = {}): LiveAgentSession {
    return {
      session_id: 'sess-abc-123',
      project_id: 'proj1',
      agent_type: 'implementor',
      workflow_id: 'feature',
      workflow_instance_id: 'inst-1',
      scheduled: false,
      duration_sec: 120,
      pid: 1234,
      rss_kb: 51200,
      cpu_pct: 10.0,
      os_uptime_sec: 300,
      ...overrides,
    }
  }

  function makeEvent(overrides: Partial<WSEventV2> = {}): WSEventV2 {
    return {
      type: 'agent.rate_limited',
      project_id: 'proj1',
      ticket_id: '',
      timestamp: '2026-01-01T00:00:00Z',
      sequence: 1,
      protocol_version: 2,
      data: {
        session_id: 'sess-abc-123',
        wait_seconds: 60,
        total_wait_seconds: 120,
        matched_pattern: 'claude-3.*rate',
        retry_count: 2,
      },
      ...overrides,
    }
  }

  describe('cache patch when session exists', () => {
    it('sets rate_limit_until_ts to now + wait_seconds', () => {
      const cacheKey = agentSessionLogKeys.live('proj1')
      qc.setQueryData<LiveAgentSessionsResponse>(cacheKey, { sessions: [makeSession()] })

      dispatchV2Event(makeEvent(), qc)

      const updated = qc.getQueryData<LiveAgentSessionsResponse>(cacheKey)
      expect(updated?.sessions[0].rate_limit_until_ts).toBe('2026-01-01T00:01:00.000Z')
    })

    it('sets rate_limit_wait_seconds', () => {
      const cacheKey = agentSessionLogKeys.live('proj1')
      qc.setQueryData<LiveAgentSessionsResponse>(cacheKey, { sessions: [makeSession()] })

      dispatchV2Event(makeEvent(), qc)

      const updated = qc.getQueryData<LiveAgentSessionsResponse>(cacheKey)
      expect(updated?.sessions[0].rate_limit_wait_seconds).toBe(60)
    })

    it('sets rate_limit_matched_pattern', () => {
      const cacheKey = agentSessionLogKeys.live('proj1')
      qc.setQueryData<LiveAgentSessionsResponse>(cacheKey, { sessions: [makeSession()] })

      dispatchV2Event(makeEvent(), qc)

      const updated = qc.getQueryData<LiveAgentSessionsResponse>(cacheKey)
      expect(updated?.sessions[0].rate_limit_matched_pattern).toBe('claude-3.*rate')
    })

    it('sets rate_limit_retry_count', () => {
      const cacheKey = agentSessionLogKeys.live('proj1')
      qc.setQueryData<LiveAgentSessionsResponse>(cacheKey, { sessions: [makeSession()] })

      dispatchV2Event(makeEvent(), qc)

      const updated = qc.getQueryData<LiveAgentSessionsResponse>(cacheKey)
      expect(updated?.sessions[0].rate_limit_retry_count).toBe(2)
    })

    it('does not patch other sessions in the same response', () => {
      const s1 = makeSession({ session_id: 'sess-abc-123' })
      const s2 = makeSession({ session_id: 'sess-other-456' })
      const cacheKey = agentSessionLogKeys.live('proj1')
      qc.setQueryData<LiveAgentSessionsResponse>(cacheKey, { sessions: [s1, s2] })

      dispatchV2Event(makeEvent(), qc)

      const updated = qc.getQueryData<LiveAgentSessionsResponse>(cacheKey)
      expect(updated?.sessions[1].rate_limit_until_ts).toBeUndefined()
      expect(updated?.sessions[1].session_id).toBe('sess-other-456')
    })
  })

  describe('cache miss fallback', () => {
    it('calls invalidateQueries on agent-session-logs when cache is empty', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent(), qc)

      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0]?.queryKey))
      expect(keys.some(k => k.includes('agent-session-logs'))).toBe(true)
    })
  })

  describe('missing data guards', () => {
    it('skips setQueryData when session_id is absent', () => {
      const cacheKey = agentSessionLogKeys.live('proj1')
      qc.setQueryData<LiveAgentSessionsResponse>(cacheKey, { sessions: [makeSession()] })
      const spy = vi.spyOn(qc, 'setQueryData')

      dispatchV2Event(makeEvent({ data: { wait_seconds: 60 } }), qc)

      expect(spy).not.toHaveBeenCalled()
    })

    it('skips setQueryData when wait_seconds is absent', () => {
      const cacheKey = agentSessionLogKeys.live('proj1')
      qc.setQueryData<LiveAgentSessionsResponse>(cacheKey, { sessions: [makeSession()] })
      const spy = vi.spyOn(qc, 'setQueryData')

      dispatchV2Event(makeEvent({ data: { session_id: 'sess-abc-123' } }), qc)

      expect(spy).not.toHaveBeenCalled()
    })
  })

  it('event type is handled (dispatchV2Event returns true)', () => {
    const handled = dispatchV2Event(makeEvent(), qc)
    expect(handled).toBe(true)
  })

  it('duplicate sequence is deduplicated (returns false)', () => {
    const event = makeEvent({ sequence: 99 })
    dispatchV2Event(event, qc)
    const handled = dispatchV2Event(event, qc)
    expect(handled).toBe(false)
  })
})
