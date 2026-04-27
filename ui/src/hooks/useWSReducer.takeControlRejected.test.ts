import { describe, it, expect, beforeEach, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

import { clearSeqs, dispatchV2Event } from './useWSReducer'
import { toast } from 'sonner'
import type { WSEventV2 } from './useWSProtocol'
import type { WSEventType } from './useWebSocket'

describe('useWSReducer — agent.take_control_rejected handler', () => {
  let qc: QueryClient

  beforeEach(() => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    clearSeqs()
    vi.clearAllMocks()
  })

  function makeEvent(overrides: Partial<WSEventV2> = {}): WSEventV2 {
    return {
      type: 'agent.take_control_rejected' as WSEventType,
      project_id: 'proj1',
      ticket_id: 'tick1',
      timestamp: '2026-04-27T00:00:00Z',
      sequence: 1,
      protocol_version: 2,
      data: { session_id: 'sess-abc', reason: 'api_mode_unsupported' },
      ...overrides,
    }
  }

  it('calls toast.error with the API-mode message', () => {
    dispatchV2Event(makeEvent(), qc)
    expect(toast.error).toHaveBeenCalledWith(
      'Take-control is not supported for API-mode agents.'
    )
  })

  it('does NOT invalidate any queries', () => {
    const spy = vi.spyOn(qc, 'invalidateQueries')
    dispatchV2Event(makeEvent(), qc)
    expect(spy).not.toHaveBeenCalled()
  })

  it('event is handled (returns true)', () => {
    const handled = dispatchV2Event(makeEvent({ sequence: 50 }), qc)
    expect(handled).toBe(true)
  })

  it('duplicate event is deduplicated — toast called only once', () => {
    const event = makeEvent({ sequence: 99 })
    dispatchV2Event(event, qc)
    const handled2 = dispatchV2Event(event, qc)
    expect(handled2).toBe(false)
    expect(vi.mocked(toast.error)).toHaveBeenCalledTimes(1)
  })

  it('subsequent events with higher seq each fire toast.error', () => {
    dispatchV2Event(makeEvent({ sequence: 1 }), qc)
    dispatchV2Event(makeEvent({ sequence: 2 }), qc)
    expect(vi.mocked(toast.error)).toHaveBeenCalledTimes(2)
  })

  it('agent.take_control_rejected is a recognised WSEventType', () => {
    const eventType: WSEventType = 'agent.take_control_rejected'
    expect(eventType).toBe('agent.take_control_rejected')
  })

  it('project-scoped event (empty ticket_id) still fires toast.error', () => {
    dispatchV2Event(makeEvent({ ticket_id: '', sequence: 10 }), qc)
    expect(toast.error).toHaveBeenCalledWith(
      'Take-control is not supported for API-mode agents.'
    )
  })

  it('project-scoped event does NOT invalidate any queries', () => {
    const spy = vi.spyOn(qc, 'invalidateQueries')
    dispatchV2Event(makeEvent({ ticket_id: '', sequence: 11 }), qc)
    expect(spy).not.toHaveBeenCalled()
  })
})
