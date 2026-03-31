import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import { projectWorkflowKeys } from './useTickets'
import type { WSEventV2 } from './useWSProtocol'

describe('useWSReducer - project_findings.updated', () => {
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

  it('invalidates project findings query key', () => {
    const event: WSEventV2 = {
      type: 'project_findings.updated',
      project_id: 'proj1',
      ticket_id: '',
      timestamp: '2026-01-01T00:00:00Z',
      sequence: 1,
      protocol_version: 2,
    }

    dispatchV2Event(event, queryClient)

    const calls = spy.mock.calls
    expect(calls.some((call: any) =>
      JSON.stringify(call[0].queryKey).includes('findings') &&
      JSON.stringify(call[0].queryKey).includes('proj1')
    )).toBe(true)
  })

  it('also invalidates project workflow query key', () => {
    const event: WSEventV2 = {
      type: 'project_findings.updated',
      project_id: 'proj2',
      ticket_id: '',
      timestamp: '2026-01-01T00:00:00Z',
      sequence: 1,
      protocol_version: 2,
    }

    dispatchV2Event(event, queryClient)

    const workflowKey = JSON.stringify(projectWorkflowKeys.workflow('proj2'))
    const calls = spy.mock.calls
    expect(calls.some((call: any) =>
      JSON.stringify(call[0].queryKey) === workflowKey
    )).toBe(true)
  })

  it('invalidates findings key matching projectWorkflowKeys.findings()', () => {
    const event: WSEventV2 = {
      type: 'project_findings.updated',
      project_id: 'my-project',
      ticket_id: '',
      timestamp: '2026-01-01T00:00:00Z',
      sequence: 1,
      protocol_version: 2,
    }

    dispatchV2Event(event, queryClient)

    const expectedKey = JSON.stringify(projectWorkflowKeys.findings('my-project'))
    const calls = spy.mock.calls
    expect(calls.some((call: any) =>
      JSON.stringify(call[0].queryKey) === expectedKey
    )).toBe(true)
  })
})
