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

describe('useWSReducer - api-mode events', () => {
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

  describe('review.created', () => {
    it('invalidates review queries', () => {
      dispatchV2Event(makeEvent('review.created'), queryClient)
      expect(
        spy.mock.calls.some((call: any) =>
          JSON.stringify(call[0].queryKey) === JSON.stringify(['review'])
        )
      ).toBe(true)
    })

    it('only invalidates once per unique sequence', () => {
      dispatchV2Event(makeEvent('review.created', 10), queryClient)
      dispatchV2Event(makeEvent('review.created', 10), queryClient)
      const matches = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['review'])
      )
      expect(matches).toHaveLength(1)
    })
  })

  describe('review.updated', () => {
    it('invalidates review queries', () => {
      dispatchV2Event(makeEvent('review.updated', 2), queryClient)
      expect(
        spy.mock.calls.some((call: any) =>
          JSON.stringify(call[0].queryKey) === JSON.stringify(['review'])
        )
      ).toBe(true)
    })
  })

  describe('config_file.updated', () => {
    it('invalidates config-files queries', () => {
      dispatchV2Event(makeEvent('config_file.updated', 3), queryClient)
      expect(
        spy.mock.calls.some((call: any) =>
          JSON.stringify(call[0].queryKey) === JSON.stringify(['config-files'])
        )
      ).toBe(true)
    })
  })

  describe('tool.dispatched - throttle', () => {
    it('fires immediately on leading edge (first dispatch)', () => {
      dispatchV2Event(makeEvent('tool.dispatched', 100), queryClient)
      const matches = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['insights'])
      )
      expect(matches).toHaveLength(1)
    })

    it('does not fire again for calls within the 1s window', () => {
      dispatchV2Event(makeEvent('tool.dispatched', 101), queryClient)
      dispatchV2Event(makeEvent('tool.dispatched', 102), queryClient)
      dispatchV2Event(makeEvent('tool.dispatched', 103), queryClient)
      const matches = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['insights'])
      )
      // Only the leading-edge call
      expect(matches).toHaveLength(1)
    })

    it('fires trailing edge after 1s window when additional calls arrived', () => {
      dispatchV2Event(makeEvent('tool.dispatched', 200), queryClient)
      dispatchV2Event(makeEvent('tool.dispatched', 201), queryClient)
      const beforeAdvance = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['insights'])
      ).length
      expect(beforeAdvance).toBe(1)

      vi.advanceTimersByTime(1000)

      const afterAdvance = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['insights'])
      ).length
      expect(afterAdvance).toBe(2)
    })

    it('does not fire trailing edge if no additional calls arrived', () => {
      dispatchV2Event(makeEvent('tool.dispatched', 300), queryClient)
      vi.advanceTimersByTime(1000)
      const matches = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['insights'])
      )
      expect(matches).toHaveLength(1)
    })

    it('re-opens window after timer expires, allowing new leading edge', () => {
      dispatchV2Event(makeEvent('tool.dispatched', 400), queryClient)
      vi.advanceTimersByTime(1000)
      spy.mockClear()
      dispatchV2Event(makeEvent('tool.dispatched', 401), queryClient)
      const matches = spy.mock.calls.filter((call: any) =>
        JSON.stringify(call[0].queryKey) === JSON.stringify(['insights'])
      )
      expect(matches).toHaveLength(1)
    })
  })
})
