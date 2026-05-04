import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import type { WSEventV2 } from './useWSProtocol'

function makeEvent(type: string, seq = 1): WSEventV2 {
  return {
    type: type as WSEventV2['type'],
    project_id: 'proj1',
    ticket_id: '',
    timestamp: '2026-01-01T00:00:00Z',
    sequence: seq,
    protocol_version: 2,
  }
}

describe('useWSReducer - nrvapp events', () => {
  let queryClient: QueryClient
  let spy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    vi.useFakeTimers()
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    spy = vi.spyOn(queryClient, 'invalidateQueries')
    clearSeqs()
    sessionStorage.clear()
  })

  afterEach(() => {
    vi.runAllTimers()
    vi.useRealTimers()
    clearSeqs()
  })

  describe('nrvapp.review_created', () => {
    it('invalidates nrvapp review queries', () => {
      dispatchV2Event(makeEvent('nrvapp.review_created'), queryClient)
      expect(
        spy.mock.calls.some((call: any) =>
          JSON.stringify(call[0].queryKey) === JSON.stringify(['nrvapp', 'review'])
        )
      ).toBe(true)
    })

    it('only invalidates once per unique sequence', () => {
      dispatchV2Event(makeEvent('nrvapp.review_created', 10), queryClient)
      dispatchV2Event(makeEvent('nrvapp.review_created', 10), queryClient)
      const matches = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['nrvapp', 'review'])
      )
      expect(matches).toHaveLength(1)
    })
  })

  describe('nrvapp.review_updated', () => {
    it('invalidates nrvapp review queries', () => {
      dispatchV2Event(makeEvent('nrvapp.review_updated', 2), queryClient)
      expect(
        spy.mock.calls.some((call: any) =>
          JSON.stringify(call[0].queryKey) === JSON.stringify(['nrvapp', 'review'])
        )
      ).toBe(true)
    })
  })

  describe('nrvapp.config_updated', () => {
    it('invalidates nrvapp config queries', () => {
      dispatchV2Event(makeEvent('nrvapp.config_updated', 3), queryClient)
      expect(
        spy.mock.calls.some((call: any) =>
          JSON.stringify(call[0].queryKey) === JSON.stringify(['nrvapp', 'config'])
        )
      ).toBe(true)
    })
  })

  describe('nrvapp.dispatch_completed - throttle', () => {
    it('fires immediately on leading edge (first dispatch)', () => {
      dispatchV2Event(makeEvent('nrvapp.dispatch_completed', 100), queryClient)
      const matches = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['nrvapp', 'insights'])
      )
      expect(matches).toHaveLength(1)
    })

    it('does not fire again for calls within the 1s window', () => {
      dispatchV2Event(makeEvent('nrvapp.dispatch_completed', 101), queryClient)
      dispatchV2Event(makeEvent('nrvapp.dispatch_completed', 102), queryClient)
      dispatchV2Event(makeEvent('nrvapp.dispatch_completed', 103), queryClient)
      const matches = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['nrvapp', 'insights'])
      )
      // Only the leading-edge call
      expect(matches).toHaveLength(1)
    })

    it('fires trailing edge after 1s window when additional calls arrived', () => {
      dispatchV2Event(makeEvent('nrvapp.dispatch_completed', 200), queryClient)
      dispatchV2Event(makeEvent('nrvapp.dispatch_completed', 201), queryClient)
      const beforeAdvance = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['nrvapp', 'insights'])
      ).length
      expect(beforeAdvance).toBe(1)

      vi.advanceTimersByTime(1000)

      const afterAdvance = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['nrvapp', 'insights'])
      ).length
      expect(afterAdvance).toBe(2)
    })

    it('does not fire trailing edge if no additional calls arrived', () => {
      dispatchV2Event(makeEvent('nrvapp.dispatch_completed', 300), queryClient)
      vi.advanceTimersByTime(1000)
      const matches = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['nrvapp', 'insights'])
      )
      expect(matches).toHaveLength(1)
    })

    it('re-opens window after timer expires, allowing new leading edge', () => {
      dispatchV2Event(makeEvent('nrvapp.dispatch_completed', 400), queryClient)
      vi.advanceTimersByTime(1000)
      spy.mockClear()
      dispatchV2Event(makeEvent('nrvapp.dispatch_completed', 401), queryClient)
      const matches = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['nrvapp', 'insights'])
      )
      expect(matches).toHaveLength(1)
    })
  })
})
