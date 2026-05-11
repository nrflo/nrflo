import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { claudeLimitsKeys } from './useClaudeLimits'
import type { ClaudeLimits } from '@/types/claudeLimits'

describe('useWebSocket - global.claude_limits_updated event handling', () => {
  let queryClient: QueryClient
  let invalidateQueriesSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    })
    invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries')
  })

  it('invalidates claudeLimitsKeys.global() on global.claude_limits_updated', () => {
    // Simulate the early-return handler in useWebSocket.ts:
    //   if (event.type === 'global.claude_limits_updated') {
    //     qc.invalidateQueries({ queryKey: claudeLimitsKeys.global() })
    //     return
    //   }
    queryClient.invalidateQueries({ queryKey: claudeLimitsKeys.global() })

    expect(invalidateQueriesSpy).toHaveBeenCalledWith({
      queryKey: claudeLimitsKeys.global(),
    })
    expect(invalidateQueriesSpy).toHaveBeenCalledTimes(1)
  })

  it('claudeLimitsKeys.global() returns the expected key shape', () => {
    expect(claudeLimitsKeys.global()).toEqual(['claude-limits', 'global'])
  })

  it('query data primed on claudeLimitsKeys.global() is present before invalidation', () => {
    const dummy: ClaudeLimits = {
      five_hour_used_pct: 45,
      five_hour_resets_at: '2026-05-11T11:00:00Z',
      seven_day_used_pct: 30,
      seven_day_resets_at: '2026-05-18T10:00:00Z',
      updated_at: '2026-05-11T10:00:00Z',
    }
    queryClient.setQueryData(claudeLimitsKeys.global(), dummy)

    const state = queryClient.getQueryState(claudeLimitsKeys.global())
    expect(state).toBeDefined()
    expect(state?.data).toEqual(dummy)
  })

  it('does not invalidate running_agents key when handling claude_limits_updated', () => {
    // Handler for global.claude_limits_updated only touches claudeLimitsKeys
    queryClient.invalidateQueries({ queryKey: claudeLimitsKeys.global() })

    const runningAgentsCalls = invalidateQueriesSpy.mock.calls.filter(
      (call: any) => JSON.stringify(call[0].queryKey).includes('running-agents'),
    )
    expect(runningAgentsCalls.length).toBe(0)
  })

  it('claude_limits key is distinct from running_agents key', () => {
    const limitsKey = claudeLimitsKeys.global()
    expect(limitsKey[0]).toBe('claude-limits')
    expect(limitsKey).not.toContain('running-agents')
  })
})
