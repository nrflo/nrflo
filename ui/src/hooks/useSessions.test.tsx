import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useSessions, sessionKeys } from './useSessions'
import * as sessionsApi from '@/api/sessions'

vi.mock('@/api/sessions')

let projectsLoaded = true
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) => selector({ currentProject: 'p', projectsLoaded })),
}))

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

const emptyList = { sessions: [] }

beforeEach(() => {
  projectsLoaded = true
  vi.mocked(sessionsApi.listSessions).mockReset()
  vi.mocked(sessionsApi.listGlobalSessions).mockReset()
})

describe('sessionKeys', () => {
  it('namespaces by scope and limit, defaulting limit to 0', () => {
    expect(sessionKeys.list('project', {})).toEqual(['sessions', 'project', 0])
    expect(sessionKeys.list('global', { limit: 50 })).toEqual(['sessions', 'global', 50])
  })
})

describe('useSessions', () => {
  it('calls listSessions for the project scope', async () => {
    vi.mocked(sessionsApi.listSessions).mockResolvedValue(emptyList)
    const { result } = renderHook(() => useSessions('project', { limit: 20 }), {
      wrapper: createWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(sessionsApi.listSessions).toHaveBeenCalledWith({ limit: 20 })
    expect(sessionsApi.listGlobalSessions).not.toHaveBeenCalled()
  })

  it('calls listGlobalSessions for the global scope', async () => {
    vi.mocked(sessionsApi.listGlobalSessions).mockResolvedValue(emptyList)
    const { result } = renderHook(() => useSessions('global'), { wrapper: createWrapper() })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(sessionsApi.listGlobalSessions).toHaveBeenCalledWith({})
    expect(sessionsApi.listSessions).not.toHaveBeenCalled()
  })

  it('is disabled until projects are loaded', () => {
    projectsLoaded = false
    const { result } = renderHook(() => useSessions('project'), { wrapper: createWrapper() })
    expect(result.current.fetchStatus).toBe('idle')
    expect(sessionsApi.listSessions).not.toHaveBeenCalled()
  })
})
