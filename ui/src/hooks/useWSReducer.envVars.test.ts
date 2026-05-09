import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import { projectEnvVarKeys } from './useProjectEnvVars'
import type { WSEventV2 } from './useWSProtocol'

function makeEnvVarsEvent(projectId: string, seq = 1): WSEventV2 {
  return {
    type: 'project.env_vars_updated',
    project_id: projectId,
    ticket_id: '',
    timestamp: '2026-01-01T00:00:00Z',
    sequence: seq,
    protocol_version: 2,
  }
}

describe('useWSReducer - project.env_vars_updated', () => {
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

  it('invalidates projectEnvVarKeys.list for the event project_id', () => {
    dispatchV2Event(makeEnvVarsEvent('proj1'), queryClient)

    const expectedKey = JSON.stringify(projectEnvVarKeys.list('proj1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === expectedKey)
    ).toBe(true)
  })

  it('does not invalidate a different project id key', () => {
    dispatchV2Event(makeEnvVarsEvent('proj2'), queryClient)

    const proj1Key = JSON.stringify(projectEnvVarKeys.list('proj1'))
    expect(
      spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === proj1Key)
    ).toBe(false)
  })
})
