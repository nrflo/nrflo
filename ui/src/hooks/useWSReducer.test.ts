import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import {
  getLastSeq,
  setLastSeq,
  getAllSeqs,
  clearSeqs,
  persistSeqs,
  restoreSeqs,
  checkSeq,
  dispatchV2Event,
  type GapResult,
} from './useWSReducer'
import type { WSEventV2 } from './useWSProtocol'

describe('useWSReducer', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    })
    clearSeqs()
    sessionStorage.clear()
  })

  afterEach(() => {
    clearSeqs()
    sessionStorage.clear()
  })

  describe('seq tracking', () => {
    it('getLastSeq returns undefined for unknown subscription', () => {
      expect(getLastSeq('proj1:tick1')).toBeUndefined()
    })

    it('setLastSeq and getLastSeq work correctly', () => {
      setLastSeq('proj1:tick1', 42)
      expect(getLastSeq('proj1:tick1')).toBe(42)
    })

    it('setLastSeq overwrites previous value', () => {
      setLastSeq('proj1:tick1', 10)
      setLastSeq('proj1:tick1', 20)
      expect(getLastSeq('proj1:tick1')).toBe(20)
    })

    it('getAllSeqs returns all tracked sequences', () => {
      setLastSeq('proj1:tick1', 10)
      setLastSeq('proj2:tick2', 20)
      const all = getAllSeqs()
      expect(all.get('proj1:tick1')).toBe(10)
      expect(all.get('proj2:tick2')).toBe(20)
      expect(all.size).toBe(2)
    })

    it('clearSeqs removes all sequences', () => {
      setLastSeq('proj1:tick1', 10)
      setLastSeq('proj2:tick2', 20)
      clearSeqs()
      expect(getLastSeq('proj1:tick1')).toBeUndefined()
      expect(getLastSeq('proj2:tick2')).toBeUndefined()
      expect(getAllSeqs().size).toBe(0)
    })
  })

  describe('sessionStorage persistence', () => {
    it('persistSeqs saves to sessionStorage', () => {
      setLastSeq('proj1:tick1', 100)
      setLastSeq('proj2:tick2', 200)
      persistSeqs()

      const stored = sessionStorage.getItem('ws_last_seqs')
      expect(stored).toBeDefined()
      const parsed = JSON.parse(stored!)
      expect(parsed['proj1:tick1']).toBe(100)
      expect(parsed['proj2:tick2']).toBe(200)
    })

    it('restoreSeqs loads from sessionStorage', () => {
      sessionStorage.setItem('ws_last_seqs', JSON.stringify({
        'proj1:tick1': 150,
        'proj3:tick3': 300,
      }))

      clearSeqs()
      restoreSeqs()

      expect(getLastSeq('proj1:tick1')).toBe(150)
      expect(getLastSeq('proj3:tick3')).toBe(300)
    })

    it('restoreSeqs handles missing storage gracefully', () => {
      sessionStorage.clear()
      restoreSeqs()
      expect(getAllSeqs().size).toBe(0)
    })

    it('restoreSeqs handles invalid JSON gracefully', () => {
      sessionStorage.setItem('ws_last_seqs', 'not valid json')
      restoreSeqs()
      expect(getAllSeqs().size).toBe(0)
    })

    it('persistSeqs handles quota exceeded gracefully', () => {
      // Mock setItem to throw
      const originalSetItem = Storage.prototype.setItem
      Storage.prototype.setItem = vi.fn(() => {
        throw new Error('QuotaExceededError')
      })

      setLastSeq('proj1:tick1', 100)
      expect(() => persistSeqs()).not.toThrow()

      Storage.prototype.setItem = originalSetItem
    })
  })

  describe('checkSeq - gap detection', () => {
    it('returns ok for first event', () => {
      const result = checkSeq('proj1:tick1', 1)
      expect(result.type).toBe('ok')
    })

    it('returns ok for sequential events', () => {
      setLastSeq('proj1:tick1', 10)
      const result = checkSeq('proj1:tick1', 11)
      expect(result.type).toBe('ok')
    })

    it('returns ok for any seq > last (gap is acceptable)', () => {
      setLastSeq('proj1:tick1', 10)
      const result = checkSeq('proj1:tick1', 15)
      expect(result.type).toBe('ok')
    })

    it('returns duplicate for same seq', () => {
      setLastSeq('proj1:tick1', 10)
      const result = checkSeq('proj1:tick1', 10)
      expect(result.type).toBe('duplicate')
    })

    it('returns duplicate for lower seq', () => {
      setLastSeq('proj1:tick1', 10)
      const result = checkSeq('proj1:tick1', 9)
      expect(result.type).toBe('duplicate')
    })

    it('different subscriptions are tracked independently', () => {
      setLastSeq('proj1:tick1', 10)
      setLastSeq('proj1:tick2', 5)

      expect(checkSeq('proj1:tick1', 11).type).toBe('ok')
      expect(checkSeq('proj1:tick2', 6).type).toBe('ok')
      expect(checkSeq('proj1:tick1', 10).type).toBe('duplicate')
      expect(checkSeq('proj1:tick2', 5).type).toBe('duplicate')
    })
  })

  describe('dispatchV2Event - idempotency', () => {
    it('accepts first event and updates seq', () => {
      const event: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      const handled = dispatchV2Event(event, queryClient)
      expect(handled).toBe(true)
      expect(getLastSeq('proj1:tick1')).toBe(1)
    })

    it('skips duplicate event', () => {
      const event: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 10,
        protocol_version: 2,
      }

      // First dispatch
      const handled1 = dispatchV2Event(event, queryClient)
      expect(handled1).toBe(true)
      expect(getLastSeq('proj1:tick1')).toBe(10)

      // Duplicate dispatch
      const handled2 = dispatchV2Event(event, queryClient)
      expect(handled2).toBe(false)
      expect(getLastSeq('proj1:tick1')).toBe(10)
    })

    it('accepts events in order and updates seq', () => {
      const events: WSEventV2[] = [
        {
          type: 'agent.started',
          project_id: 'proj1',
          ticket_id: 'tick1',
          timestamp: '2026-02-14T00:00:00Z',
          sequence: 1,
          protocol_version: 2,
        },
        {
          type: 'agent.completed',
          project_id: 'proj1',
          ticket_id: 'tick1',
          timestamp: '2026-02-14T00:00:01Z',
          sequence: 2,
          protocol_version: 2,
        },
        {
          type: 'workflow.updated',
          project_id: 'proj1',
          ticket_id: 'tick1',
          timestamp: '2026-02-14T00:00:02Z',
          sequence: 3,
          protocol_version: 2,
        },
      ]

      events.forEach((event, i) => {
        const handled = dispatchV2Event(event, queryClient)
        expect(handled).toBe(true)
        expect(getLastSeq('proj1:tick1')).toBe(i + 1)
      })
    })

    it('handles event without sequence (v1 backward compat)', () => {
      const event: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        // No sequence field
      }

      const handled = dispatchV2Event(event, queryClient)
      expect(handled).toBe(true)
      expect(getLastSeq('proj1:tick1')).toBeUndefined()
    })
  })

  describe('dispatchV2Event - event routing', () => {
    beforeEach(() => {
      // Spy on invalidateQueries
      vi.spyOn(queryClient, 'invalidateQueries')
    })

    it('routes agent.started to correct invalidations (ticket scope)', () => {
      const event: WSEventV2 = {
        type: 'agent.started',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      dispatchV2Event(event, queryClient)

      // Should invalidate ticket detail, workflow, and agent sessions
      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })

    it('routes agent.started to correct invalidations (project scope)', () => {
      const event: WSEventV2 = {
        type: 'agent.started',
        project_id: 'proj1',
        ticket_id: '', // Empty ticket_id = project scope
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      dispatchV2Event(event, queryClient)

      // Should invalidate project workflow and agent sessions
      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })

    it('routes agent.completed event', () => {
      const event: WSEventV2 = {
        type: 'agent.completed',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      dispatchV2Event(event, queryClient)
      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })

    it('routes agent.continued event', () => {
      const event: WSEventV2 = {
        type: 'agent.continued',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      dispatchV2Event(event, queryClient)
      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })

    it('routes phase.started event', () => {
      const event: WSEventV2 = {
        type: 'phase.started',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      dispatchV2Event(event, queryClient)
      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })

    it('routes findings.updated event', () => {
      const event: WSEventV2 = {
        type: 'findings.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      dispatchV2Event(event, queryClient)
      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })

    it('routes messages.updated event with session_id', () => {
      const event: WSEventV2 = {
        type: 'messages.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
        data: { session_id: 'sess-123' },
      }

      dispatchV2Event(event, queryClient)
      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })

    it('routes chain.updated event', () => {
      const event: WSEventV2 = {
        type: 'chain.updated',
        project_id: 'proj1',
        ticket_id: '',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
        data: { chain_id: 'chain-456' },
      }

      dispatchV2Event(event, queryClient)
      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })

    it('routes workflow_def.created event', () => {
      const event: WSEventV2 = {
        type: 'workflow_def.created',
        project_id: 'proj1',
        ticket_id: '',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      dispatchV2Event(event, queryClient)
      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })

    it('routes orchestration events', () => {
      const orchestrationTypes = [
        'orchestration.started',
        'orchestration.completed',
        'orchestration.failed',
        'orchestration.retried',
        'orchestration.callback',
      ] as const

      orchestrationTypes.forEach((type, i) => {
        const localQueryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
        const spy = vi.spyOn(localQueryClient, 'invalidateQueries')

        const event: WSEventV2 = {
          type,
          project_id: 'proj1',
          ticket_id: 'tick1',
          timestamp: '2026-02-14T00:00:00Z',
          sequence: 100 + i, // Unique seq per event
          protocol_version: 2,
        }

        dispatchV2Event(event, localQueryClient)
        expect(spy).toHaveBeenCalled()
      })
    })

    it('handles unknown event types gracefully', () => {
      const event: WSEventV2 = {
        type: 'unknown.event' as any,
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      expect(() => dispatchV2Event(event, queryClient)).not.toThrow()
    })
  })

  describe('dispatchV2Event - cache patches', () => {
    it('does not throw when query data is not cached yet', () => {
      const event: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      // Cache is empty - should not throw
      expect(() => dispatchV2Event(event, queryClient)).not.toThrow()
    })
  })

  describe('GapResult type', () => {
    it('ok result has correct shape', () => {
      const result: GapResult = { type: 'ok' }
      expect(result.type).toBe('ok')
    })

    it('duplicate result has correct shape', () => {
      const result: GapResult = { type: 'duplicate' }
      expect(result.type).toBe('duplicate')
    })

    it('gap result has expected and got fields', () => {
      const result: GapResult = { type: 'gap', expected: 11, got: 15 }
      expect(result.type).toBe('gap')
      expect(result.expected).toBe(11)
      expect(result.got).toBe(15)
    })
  })

  describe('orchestration events - ticket status invalidation', () => {
    beforeEach(() => {
      vi.spyOn(queryClient, 'invalidateQueries')
    })

    it('orchestration.started invalidates status, lists, and dailyStats for ticket-scoped events', () => {
      const event: WSEventV2 = {
        type: 'orchestration.started',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-15T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      dispatchV2Event(event, queryClient)

      // Verify all expected invalidations happened
      const invalidateCalls = (queryClient.invalidateQueries as any).mock.calls

      // Should invalidate workflow queries
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('workflow')
      )).toBe(true)

      // Should invalidate ticketKeys.status()
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('status')
      )).toBe(true)

      // Should invalidate ticketKeys.lists()
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('list')
      )).toBe(true)

      // Should invalidate dailyStatsKeys.all
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('daily-stats')
      )).toBe(true)
    })

    it('orchestration.completed invalidates status, lists, and dailyStats for ticket-scoped events', () => {
      const event: WSEventV2 = {
        type: 'orchestration.completed',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-15T00:00:00Z',
        sequence: 2,
        protocol_version: 2,
      }

      dispatchV2Event(event, queryClient)

      const invalidateCalls = (queryClient.invalidateQueries as any).mock.calls

      // Should invalidate workflow queries
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('workflow')
      )).toBe(true)

      // Should invalidate ticketKeys.status()
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('status')
      )).toBe(true)

      // Should invalidate ticketKeys.lists()
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('list')
      )).toBe(true)

      // Should invalidate dailyStatsKeys.all
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('daily-stats')
      )).toBe(true)
    })

    it('orchestration.failed invalidates status, lists, and dailyStats for ticket-scoped events', () => {
      const event: WSEventV2 = {
        type: 'orchestration.failed',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-15T00:00:00Z',
        sequence: 3,
        protocol_version: 2,
      }

      dispatchV2Event(event, queryClient)

      const invalidateCalls = (queryClient.invalidateQueries as any).mock.calls

      // Should invalidate workflow queries
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('workflow')
      )).toBe(true)

      // Should invalidate ticketKeys.status()
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('status')
      )).toBe(true)

      // Should invalidate ticketKeys.lists()
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('list')
      )).toBe(true)

      // Should invalidate dailyStatsKeys.all
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('daily-stats')
      )).toBe(true)
    })

    it('orchestration.started does NOT invalidate status/lists/dailyStats for project-scoped events', () => {
      const event: WSEventV2 = {
        type: 'orchestration.started',
        project_id: 'proj1',
        ticket_id: '', // Empty ticket_id = project scope
        timestamp: '2026-02-15T00:00:00Z',
        sequence: 4,
        protocol_version: 2,
      }

      const localQueryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const spy = vi.spyOn(localQueryClient, 'invalidateQueries')

      dispatchV2Event(event, localQueryClient)

      const invalidateCalls = spy.mock.calls

      // Should invalidate project workflow
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('workflow')
      )).toBe(true)

      // Should NOT invalidate ticketKeys.status()
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('tickets') &&
        JSON.stringify(call[0].queryKey).includes('status')
      )).toBe(false)

      // Should NOT invalidate ticketKeys.lists()
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('tickets') &&
        JSON.stringify(call[0].queryKey).includes('list')
      )).toBe(false)

      // Should NOT invalidate dailyStatsKeys.all
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('daily-stats')
      )).toBe(false)
    })

    it('orchestration.completed does NOT invalidate status/lists/dailyStats for project-scoped events', () => {
      const event: WSEventV2 = {
        type: 'orchestration.completed',
        project_id: 'proj1',
        ticket_id: '', // Empty ticket_id = project scope
        timestamp: '2026-02-15T00:00:00Z',
        sequence: 5,
        protocol_version: 2,
      }

      const localQueryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const spy = vi.spyOn(localQueryClient, 'invalidateQueries')

      dispatchV2Event(event, localQueryClient)

      const invalidateCalls = spy.mock.calls

      // Should invalidate project workflow
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('workflow')
      )).toBe(true)

      // Should NOT invalidate ticket-specific queries
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('tickets') &&
        JSON.stringify(call[0].queryKey).includes('status')
      )).toBe(false)

      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('tickets') &&
        JSON.stringify(call[0].queryKey).includes('list')
      )).toBe(false)

      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('daily-stats')
      )).toBe(false)
    })

    it('orchestration.failed does NOT invalidate status/lists/dailyStats for project-scoped events', () => {
      const event: WSEventV2 = {
        type: 'orchestration.failed',
        project_id: 'proj1',
        ticket_id: '', // Empty ticket_id = project scope
        timestamp: '2026-02-15T00:00:00Z',
        sequence: 6,
        protocol_version: 2,
      }

      const localQueryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const spy = vi.spyOn(localQueryClient, 'invalidateQueries')

      dispatchV2Event(event, localQueryClient)

      const invalidateCalls = spy.mock.calls

      // Should invalidate project workflow
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('workflow')
      )).toBe(true)

      // Should NOT invalidate ticket-specific queries
      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('tickets') &&
        JSON.stringify(call[0].queryKey).includes('status')
      )).toBe(false)

      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('tickets') &&
        JSON.stringify(call[0].queryKey).includes('list')
      )).toBe(false)

      expect(invalidateCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('daily-stats')
      )).toBe(false)
    })

    it('orchestration events invalidate workflow queries for both ticket and project scope', () => {
      const ticketEvent: WSEventV2 = {
        type: 'orchestration.completed',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-15T00:00:00Z',
        sequence: 7,
        protocol_version: 2,
      }

      const projectEvent: WSEventV2 = {
        type: 'orchestration.completed',
        project_id: 'proj1',
        ticket_id: '',
        timestamp: '2026-02-15T00:00:01Z',
        sequence: 8,
        protocol_version: 2,
      }

      const ticketQC = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const projectQC = new QueryClient({ defaultOptions: { queries: { retry: false } } })

      const ticketSpy = vi.spyOn(ticketQC, 'invalidateQueries')
      const projectSpy = vi.spyOn(projectQC, 'invalidateQueries')

      dispatchV2Event(ticketEvent, ticketQC)
      dispatchV2Event(projectEvent, projectQC)

      // Both should invalidate workflow queries
      expect(ticketSpy).toHaveBeenCalled()
      expect(projectSpy).toHaveBeenCalled()

      const ticketCalls = ticketSpy.mock.calls
      const projectCalls = projectSpy.mock.calls

      expect(ticketCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('workflow')
      )).toBe(true)

      expect(projectCalls.some((call: any) =>
        JSON.stringify(call[0].queryKey).includes('workflow')
      )).toBe(true)
    })

    it('multiple sequential orchestration events trigger invalidations correctly', () => {
      const events: WSEventV2[] = [
        {
          type: 'orchestration.started',
          project_id: 'proj1',
          ticket_id: 'tick1',
          timestamp: '2026-02-15T00:00:00Z',
          sequence: 10,
          protocol_version: 2,
        },
        {
          type: 'orchestration.completed',
          project_id: 'proj1',
          ticket_id: 'tick1',
          timestamp: '2026-02-15T00:00:10Z',
          sequence: 11,
          protocol_version: 2,
        },
      ]

      const localQueryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const spy = vi.spyOn(localQueryClient, 'invalidateQueries')

      events.forEach(event => {
        dispatchV2Event(event, localQueryClient)
      })

      // Both events should have triggered invalidations
      expect(spy).toHaveBeenCalled()

      // Verify each event was processed (not deduplicated)
      expect(getLastSeq('proj1:tick1')).toBe(11)
    })
  })
})
