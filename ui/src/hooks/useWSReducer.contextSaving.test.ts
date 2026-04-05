import { describe, it, expect, beforeEach, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import type { WSEventV2 } from './useWSProtocol'
import type { WSEventType } from './useWebSocket'

describe('useWSReducer — agent.context_saving handler', () => {
  let qc: QueryClient

  beforeEach(() => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    clearSeqs()
  })

  function makeEvent(overrides: Partial<WSEventV2> = {}): WSEventV2 {
    return {
      type: 'agent.context_saving' as WSEventType,
      project_id: 'proj1',
      ticket_id: 'tick1',
      timestamp: '2026-04-05T00:00:00Z',
      sequence: 1,
      protocol_version: 2,
      data: {
        session_id: 'sess-abc',
        agent_type: 'implementor',
      },
      ...overrides,
    }
  }

  describe('ticket-scoped', () => {
    it('invalidates ticketKeys.detail for the ticket', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent(), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('tick1') && k.includes('detail'))).toBe(true)
    })

    it('invalidates ticketKeys.workflow for the ticket', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent(), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('tick1') && k.includes('workflow'))).toBe(true)
    })

    it('invalidates ticketKeys.agentSessions for the ticket', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent(), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('tick1') && k.includes('agents'))).toBe(true)
    })

    it('does NOT invalidate projectWorkflowKeys for a ticket-scoped event', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent(), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('project-workflows') && !k.includes('tick1'))).toBe(false)
    })
  })

  describe('project-scoped (empty ticket_id)', () => {
    it('invalidates projectWorkflowKeys.workflow', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent({ ticket_id: '', sequence: 10 }), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('workflow'))).toBe(true)
    })

    it('invalidates projectWorkflowKeys.agentSessions', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent({ ticket_id: '', sequence: 11 }), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('agents'))).toBe(true)
    })

    it('does NOT invalidate ticketKeys.detail for project-scoped event', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      dispatchV2Event(makeEvent({ ticket_id: '', sequence: 12 }), qc)
      const keys = spy.mock.calls.map((c: any) => JSON.stringify(c[0].queryKey))
      expect(keys.some(k => k.includes('detail'))).toBe(false)
    })
  })

  describe('event type registration', () => {
    it('agent.context_saving is a recognised WSEventType (TypeScript compile check)', () => {
      const eventType: WSEventType = 'agent.context_saving'
      expect(eventType).toBe('agent.context_saving')
    })

    it('agent.context_saving event is handled (returns true, not ignored)', () => {
      const handled = dispatchV2Event(makeEvent({ sequence: 50 }), qc)
      expect(handled).toBe(true)
    })

    it('duplicate agent.context_saving events are deduplicated', () => {
      const spy = vi.spyOn(qc, 'invalidateQueries')
      const event = makeEvent({ sequence: 99 })
      dispatchV2Event(event, qc)
      const callCountAfterFirst = spy.mock.calls.length
      const handled2 = dispatchV2Event(event, qc)
      expect(handled2).toBe(false)
      expect(spy.mock.calls.length).toBe(callCountAfterFirst)
    })

    it('subsequent agent.context_saving events with higher seq are handled', () => {
      dispatchV2Event(makeEvent({ sequence: 1 }), qc)
      const spy = vi.spyOn(qc, 'invalidateQueries')
      const handled = dispatchV2Event(makeEvent({ sequence: 2 }), qc)
      expect(handled).toBe(true)
      expect(spy).toHaveBeenCalled()
    })
  })
})
