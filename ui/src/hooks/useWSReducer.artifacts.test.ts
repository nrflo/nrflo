import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { dispatchV2Event, clearSeqs } from './useWSReducer'
import { artifactKeys } from './useArtifacts'
import type { WSEventV2 } from './useWSProtocol'

describe('useWSReducer — artifact events', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    clearSeqs()
    sessionStorage.clear()
  })

  afterEach(() => {
    clearSeqs()
    sessionStorage.clear()
  })

  function makeEvent(type: string, workflowInstanceId?: string, seq = 1): WSEventV2 {
    return {
      type,
      project_id: 'proj1',
      ticket_id: '',
      timestamp: '2026-01-01T00:00:00Z',
      sequence: seq,
      protocol_version: 2,
      ...(workflowInstanceId ? { data: { workflow_instance_id: workflowInstanceId } } : {}),
    }
  }

  it('artifact.created invalidates artifactKeys.instance for the workflow instance', () => {
    const spy = vi.spyOn(queryClient, 'invalidateQueries')
    dispatchV2Event(makeEvent('artifact.created', 'wfi-abc'), queryClient)

    expect(spy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: artifactKeys.instance('wfi-abc') })
    )
  })

  it('artifact.deleted invalidates artifactKeys.instance for the workflow instance', () => {
    const spy = vi.spyOn(queryClient, 'invalidateQueries')
    dispatchV2Event(makeEvent('artifact.deleted', 'wfi-xyz'), queryClient)

    expect(spy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: artifactKeys.instance('wfi-xyz') })
    )
  })

  it('artifact.created with no workflow_instance_id does not call invalidateQueries', () => {
    const spy = vi.spyOn(queryClient, 'invalidateQueries')
    dispatchV2Event(makeEvent('artifact.created'), queryClient)

    const artifactCalls = spy.mock.calls.filter(([arg]) =>
      JSON.stringify(arg).includes('artifacts')
    )
    expect(artifactCalls).toHaveLength(0)
  })

  it('artifact.deleted with no workflow_instance_id does not call invalidateQueries', () => {
    const spy = vi.spyOn(queryClient, 'invalidateQueries')
    dispatchV2Event(makeEvent('artifact.deleted'), queryClient)

    const artifactCalls = spy.mock.calls.filter(([arg]) =>
      JSON.stringify(arg).includes('artifacts')
    )
    expect(artifactCalls).toHaveLength(0)
  })

  it('artifact events are idempotent — duplicate seq is ignored', () => {
    const spy = vi.spyOn(queryClient, 'invalidateQueries')
    const event = makeEvent('artifact.created', 'wfi-1', 10)
    dispatchV2Event(event, queryClient)
    const callCount = spy.mock.calls.length
    dispatchV2Event(event, queryClient)
    expect(spy.mock.calls.length).toBe(callCount)
  })
})
