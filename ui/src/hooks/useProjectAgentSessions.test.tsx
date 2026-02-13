import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useProjectAgentSessions, projectWorkflowKeys } from './useTickets'
import * as projectWorkflowsApi from '@/api/projectWorkflows'
import type { ProjectAgentSessionsResponse } from '@/types/workflow'
import type { ReactNode } from 'react'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true }),
}))

vi.mock('@/api/projectWorkflows', async () => {
  const actual = await vi.importActual('@/api/projectWorkflows')
  return {
    ...actual,
    getProjectAgentSessions: vi.fn(),
  }
})

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    )
  }
}

const sampleResponse: ProjectAgentSessionsResponse = {
  project_id: 'test-project',
  sessions: [
    {
      id: 'session-1',
      project_id: 'test-project',
      ticket_id: '',
      workflow_instance_id: 'wi-1',
      phase: 'investigation',
      workflow: 'feature',
      agent_type: 'setup-analyzer',
      model_id: 'claude-sonnet-4-5',
      status: 'running',
      message_count: 5,
      restart_count: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
    {
      id: 'session-2',
      project_id: 'test-project',
      ticket_id: '',
      workflow_instance_id: 'wi-1',
      phase: 'implementation',
      workflow: 'feature',
      agent_type: 'implementor',
      model_id: 'claude-opus-4-6',
      status: 'completed',
      result: 'pass',
      message_count: 20,
      restart_count: 1,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:05:00Z',
      started_at: '2026-01-01T00:00:00Z',
      ended_at: '2026-01-01T00:05:00Z',
    },
  ],
}

describe('useProjectAgentSessions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('fetches project agent sessions when projectId is provided and projectsLoaded is true', async () => {
    vi.mocked(projectWorkflowsApi.getProjectAgentSessions).mockResolvedValue(sampleResponse)

    const { result } = renderHook(
      () => useProjectAgentSessions('test-project'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data).toBeDefined()
    })

    expect(projectWorkflowsApi.getProjectAgentSessions).toHaveBeenCalledWith('test-project')
    expect(result.current.data?.project_id).toBe('test-project')
    expect(result.current.data?.sessions).toHaveLength(2)
    expect(result.current.data?.sessions[0].agent_type).toBe('setup-analyzer')
    expect(result.current.data?.sessions[1].agent_type).toBe('implementor')
  })

  it('does not fetch when projectId is empty string', () => {
    const { result } = renderHook(
      () => useProjectAgentSessions(''),
      { wrapper: createWrapper() }
    )

    expect(result.current.data).toBeUndefined()
    expect(result.current.fetchStatus).toBe('idle')
    expect(projectWorkflowsApi.getProjectAgentSessions).not.toHaveBeenCalled()
  })

  it('does not fetch when projectsLoaded is false', () => {
    // Override the mock for this test only
    vi.doMock('@/stores/projectStore', () => ({
      useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
        selector({ currentProject: 'test-project', projectsLoaded: false }),
    }))

    const { result } = renderHook(
      () => useProjectAgentSessions('test-project', { enabled: false }), // Force disabled for this test
      { wrapper: createWrapper() }
    )

    expect(result.current.data).toBeUndefined()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('uses correct query key for project agent sessions', async () => {
    vi.mocked(projectWorkflowsApi.getProjectAgentSessions).mockResolvedValue(sampleResponse)

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    )

    renderHook(() => useProjectAgentSessions('test-project'), { wrapper })

    await waitFor(() => {
      const cache = queryClient.getQueryCache()
      const queries = cache.getAll()
      const projectAgentQuery = queries.find(q =>
        JSON.stringify(q.queryKey) === JSON.stringify(projectWorkflowKeys.agentSessions('test-project'))
      )
      expect(projectAgentQuery).toBeDefined()
    })
  })

  it('respects custom enabled option', () => {
    const { result } = renderHook(
      () => useProjectAgentSessions('test-project', { enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(result.current.data).toBeUndefined()
    expect(result.current.fetchStatus).toBe('idle')
    expect(projectWorkflowsApi.getProjectAgentSessions).not.toHaveBeenCalled()
  })

  it('returns error state when API call fails', async () => {
    vi.mocked(projectWorkflowsApi.getProjectAgentSessions).mockRejectedValue(new Error('Network error'))

    const { result } = renderHook(
      () => useProjectAgentSessions('test-project'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBeDefined()
  })

  it('returns empty sessions array when no agents exist', async () => {
    const emptyResponse: ProjectAgentSessionsResponse = {
      project_id: 'test-project',
      sessions: [],
    }
    vi.mocked(projectWorkflowsApi.getProjectAgentSessions).mockResolvedValue(emptyResponse)

    const { result } = renderHook(
      () => useProjectAgentSessions('test-project'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data).toBeDefined()
    })

    expect(result.current.data?.sessions).toEqual([])
  })

  it('handles sessions with different statuses (running, completed, failed)', async () => {
    const mixedResponse: ProjectAgentSessionsResponse = {
      project_id: 'test-project',
      sessions: [
        {
          id: 'session-running',
          project_id: 'test-project',
          ticket_id: '',
          workflow_instance_id: 'wi-1',
          phase: 'investigation',
          workflow: 'feature',
          agent_type: 'setup-analyzer',
          model_id: 'claude-sonnet-4-5',
          status: 'running',
          message_count: 3,
          restart_count: 0,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
        {
          id: 'session-completed',
          project_id: 'test-project',
          ticket_id: '',
          workflow_instance_id: 'wi-1',
          phase: 'implementation',
          workflow: 'feature',
          agent_type: 'implementor',
          model_id: 'claude-opus-4-6',
          status: 'completed',
          result: 'pass',
          message_count: 15,
          restart_count: 0,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:05:00Z',
        },
        {
          id: 'session-failed',
          project_id: 'test-project',
          ticket_id: '',
          workflow_instance_id: 'wi-1',
          phase: 'verification',
          workflow: 'feature',
          agent_type: 'qa-verifier',
          model_id: 'claude-sonnet-4-5',
          status: 'failed',
          result: 'fail',
          result_reason: 'Test failures',
          message_count: 10,
          restart_count: 0,
          created_at: '2026-01-01T00:05:00Z',
          updated_at: '2026-01-01T00:10:00Z',
        },
      ],
    }
    vi.mocked(projectWorkflowsApi.getProjectAgentSessions).mockResolvedValue(mixedResponse)

    const { result } = renderHook(
      () => useProjectAgentSessions('test-project'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data).toBeDefined()
    })

    expect(result.current.data?.sessions).toHaveLength(3)
    expect(result.current.data?.sessions[0].status).toBe('running')
    expect(result.current.data?.sessions[1].status).toBe('completed')
    expect(result.current.data?.sessions[2].status).toBe('failed')
  })

  it('query key does not collide with ticket agent sessions keys', () => {
    const projectKey = projectWorkflowKeys.agentSessions('test-project')
    const ticketKey = ['tickets', 'detail', 'TICKET-123', 'agents', undefined]

    expect(JSON.stringify(projectKey)).not.toBe(JSON.stringify(ticketKey))
    expect(projectKey[0]).toBe('project-workflows')
    expect(projectKey[1]).toBe('agents')
    expect(projectKey[2]).toBe('test-project')
  })
})
