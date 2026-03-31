import { describe, it, expect, beforeEach, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import type { WSEventV2 } from './useWSProtocol'
import type { WSEventType } from './useWebSocket'

describe('useWSReducer — merge conflict handlers', () => {
  let qc: QueryClient

  beforeEach(() => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    clearSeqs()
  })

  function makeEvent(type: WSEventType, seq: number, overrides: Partial<WSEventV2> = {}): WSEventV2 {
    return {
      type,
      project_id: 'proj1',
      ticket_id: 'tick1',
      timestamp: '2026-01-01T00:00:00Z',
      sequence: seq,
      protocol_version: 2,
      data: {},
      ...overrides,
    }
  }

  describe.each([
    ['merge.conflict_resolving' as WSEventType],
    ['merge.conflict_resolved' as WSEventType],
    ['merge.conflict_failed' as WSEventType],
  ])('%s — ticket-scoped', (eventType) => {
    it('invalidates ticketKeys.detail', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent(eventType, 1), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('tick1') && k.includes('detail'))).toBe(true)
    })

    it('invalidates ticketKeys.workflow', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent(eventType, 2), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('tick1') && k.includes('workflow'))).toBe(true)
    })

    it('invalidates ticketKeys.agentSessions', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent(eventType, 3), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('tick1') && k.includes('agents'))).toBe(true)
    })
  })

  describe.each([
    ['merge.conflict_resolving' as WSEventType],
    ['merge.conflict_resolved' as WSEventType],
    ['merge.conflict_failed' as WSEventType],
  ])('%s — project-scoped (empty ticket_id)', (eventType) => {
    it('invalidates projectWorkflowKeys.workflow', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent(eventType, 10, { ticket_id: '' }), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('workflow'))).toBe(true)
    })

    it('invalidates projectWorkflowKeys.agentSessions', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent(eventType, 11, { ticket_id: '' }), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('agents'))).toBe(true)
    })

    it('does NOT invalidate ticketKeys.detail for project-scoped event', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent(eventType, 12, { ticket_id: '' }), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('detail'))).toBe(false)
    })
  })

  describe('merge.conflict_resolving — also invalidates runningAgents', () => {
    it('invalidates runningAgentsKeys.all', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent('merge.conflict_resolving', 20), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('running'))).toBe(true)
    })

    it('merge.conflict_resolved does NOT invalidate runningAgents', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent('merge.conflict_resolved', 21), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      // Should not invalidate running agents (no new running agent)
      const runningInvalidations = keys.filter(k => k.includes('running') && !k.includes('workflow'))
      expect(runningInvalidations.length).toBe(0)
    })
  })

  describe('event type registration', () => {
    it('merge.conflict_resolving is handled (returns true)', () => {
      expect(dispatchV2Event(makeEvent('merge.conflict_resolving', 30), qc)).toBe(true)
    })

    it('merge.conflict_resolved is handled (returns true)', () => {
      expect(dispatchV2Event(makeEvent('merge.conflict_resolved', 31), qc)).toBe(true)
    })

    it('merge.conflict_failed is handled (returns true)', () => {
      expect(dispatchV2Event(makeEvent('merge.conflict_failed', 32), qc)).toBe(true)
    })

    it('duplicate events are deduplicated', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      const event = makeEvent('merge.conflict_resolving', 99)
      dispatchV2Event(event, qc)
      const firstCount = spy.mock.calls.length
      expect(dispatchV2Event(event, qc)).toBe(false)
      expect(spy.mock.calls.length).toBe(firstCount)
    })
  })
})
