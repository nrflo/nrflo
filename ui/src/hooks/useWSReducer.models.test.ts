import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { clearSeqs, dispatchV2Event } from './useWSReducer'
import { modelKeys } from './useModels'
import type { WSEventV2 } from './useWSProtocol'

function makeModelEvent(type: string, seq = 1): WSEventV2 {
  return {
    type,
    project_id: '',
    ticket_id: '',
    timestamp: '2026-01-01T00:00:00Z',
    sequence: seq,
    protocol_version: 2,
    data: { model_id: 'custom-model' },
  } as WSEventV2
}

describe('useWSReducer - model.* events', () => {
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

  it.each(['model.created', 'model.updated', 'model.deleted'])(
    'invalidates the models list query for %s',
    (type) => {
      dispatchV2Event(makeModelEvent(type), queryClient)

      const expectedKey = JSON.stringify(modelKeys.list())
      expect(
        spy.mock.calls.some((call: any) => JSON.stringify(call[0].queryKey) === expectedKey)
      ).toBe(true)
    },
  )
})
