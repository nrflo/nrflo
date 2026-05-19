import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ActiveObserversPanel } from './ActiveObserversPanel'
import { useInteractiveSessionsStore } from '@/stores/interactiveSessionsStore'
import type { WSEvent } from '@/hooks/useWebSocket'
import type { AgentSession } from '@/types/workflow'
import { renderWithQuery } from '@/test/utils'

// ---- WS mock ----
let capturedWSListener: ((e: WSEvent) => void) | null = null
const mockAddEventListener = vi.fn((fn: (e: WSEvent) => void) => {
  capturedWSListener = fn
})
const mockRemoveEventListener = vi.fn()

vi.mock('@/providers/WebSocketProvider', () => ({
  useWebSocketContext: () => ({
    isConnected: true,
    subscribe: vi.fn(),
    unsubscribe: vi.fn(),
    addEventListener: mockAddEventListener,
    removeEventListener: mockRemoveEventListener,
  }),
}))

// ---- observer hook mock ----
const mockUseObservers = vi.fn()

vi.mock('@/hooks/useObservers', () => ({
  useObservers: () => mockUseObservers(),
  observerKeys: {
    all: ['observers'],
    list: (p: string) => ['observers', p],
  },
}))

// ---- settings mock ----
const mockObserverEnabled = vi.fn()

vi.mock('@/hooks/useGlobalSettings', () => ({
  useExperimentalObserverEnabled: () => mockObserverEnabled(),
}))

// ---- project store mock ----
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string }) => unknown) =>
    selector({ currentProject: 'test-project' }),
}))

// ---- helpers ----
function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 'sess-1',
    project_id: 'test-project',
    ticket_id: '',
    workflow_instance_id: 'wfi-1',
    phase: 'observer',
    workflow: 'observer',
    agent_type: 'observer',
    status: 'running',
    message_count: 0,
    restart_count: 0,
    started_at: '2024-01-01T00:00:00Z',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeWSEvent(type: WSEvent['type'], data: Record<string, unknown>): WSEvent {
  return {
    type,
    project_id: 'test-project',
    ticket_id: '',
    timestamp: new Date().toISOString(),
    data,
  }
}

describe('ActiveObserversPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    capturedWSListener = null
    mockObserverEnabled.mockReturnValue(true)
    useInteractiveSessionsStore.setState({ sessions: [], activeId: '', minimized: false })
  })

  it('renders nothing when experimental_observer_enabled is false', () => {
    mockObserverEnabled.mockReturnValue(false)
    mockUseObservers.mockReturnValue({ data: { sessions: [makeSession()], count: 1 } })
    const { container } = renderWithQuery(<ActiveObserversPanel />)
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when sessions list is empty', () => {
    mockUseObservers.mockReturnValue({ data: { sessions: [], count: 0 } })
    const { container } = renderWithQuery(<ActiveObserversPanel />)
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when data is undefined', () => {
    mockUseObservers.mockReturnValue({ data: undefined })
    const { container } = renderWithQuery(<ActiveObserversPanel />)
    expect(container.firstChild).toBeNull()
  })

  it('renders a row for each session', () => {
    mockUseObservers.mockReturnValue({
      data: {
        sessions: [
          makeSession({ id: 'sess-1', workflow: 'my-workflow' }),
          makeSession({ id: 'sess-2', workflow: 'other-workflow' }),
        ],
        count: 2,
      },
    })
    renderWithQuery(<ActiveObserversPanel />)
    expect(screen.getByText(/my-workflow \(sess-1/)).toBeInTheDocument()
    expect(screen.getByText(/other-workflow \(sess-2/)).toBeInTheDocument()
  })

  it('renders Active Observers heading', () => {
    mockUseObservers.mockReturnValue({ data: { sessions: [makeSession()], count: 1 } })
    renderWithQuery(<ActiveObserversPanel />)
    expect(screen.getByText('Active Observers')).toBeInTheDocument()
  })

  it('renders Attach button for each session', () => {
    mockUseObservers.mockReturnValue({
      data: {
        sessions: [makeSession({ id: 'sess-1' }), makeSession({ id: 'sess-2' })],
        count: 2,
      },
    })
    renderWithQuery(<ActiveObserversPanel />)
    const attachButtons = screen.getAllByRole('button', { name: /attach/i })
    expect(attachButtons).toHaveLength(2)
  })

  it('clicking Attach adds session to interactiveSessionsStore', async () => {
    const session = makeSession({ id: 'sess-abc', workflow: 'my-obs', project_id: 'proj-x' })
    mockUseObservers.mockReturnValue({ data: { sessions: [session], count: 1 } })
    renderWithQuery(<ActiveObserversPanel />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /attach/i }))

    const sessions = useInteractiveSessionsStore.getState().sessions
    expect(sessions).toContainEqual(
      expect.objectContaining({ sessionId: 'sess-abc', agentType: 'observer' })
    )
  })

  it('WS agent.started with kind=observer triggers invalidateQueries', () => {
    mockUseObservers.mockReturnValue({ data: { sessions: [makeSession()], count: 1 } })
    const { queryClient } = renderWithQuery(<ActiveObserversPanel />)
    const spy = vi.spyOn(queryClient, 'invalidateQueries')

    act(() => {
      capturedWSListener!(makeWSEvent('agent.started', { kind: 'observer' }))
    })

    expect(spy).toHaveBeenCalledWith({ queryKey: ['observers', 'test-project'] })
  })

  it('WS agent.completed with kind=observer triggers invalidateQueries', () => {
    mockUseObservers.mockReturnValue({ data: { sessions: [makeSession()], count: 1 } })
    const { queryClient } = renderWithQuery(<ActiveObserversPanel />)
    const spy = vi.spyOn(queryClient, 'invalidateQueries')

    act(() => {
      capturedWSListener!(makeWSEvent('agent.completed', { kind: 'observer' }))
    })

    expect(spy).toHaveBeenCalled()
  })

  it('WS agent.started with kind=workflow_agent does NOT trigger invalidateQueries', () => {
    mockUseObservers.mockReturnValue({ data: { sessions: [makeSession()], count: 1 } })
    const { queryClient } = renderWithQuery(<ActiveObserversPanel />)
    const spy = vi.spyOn(queryClient, 'invalidateQueries')

    act(() => {
      capturedWSListener!(makeWSEvent('agent.started', { kind: 'workflow_agent' }))
    })

    expect(spy).not.toHaveBeenCalled()
  })

  it('WS event without kind does NOT trigger invalidateQueries', () => {
    mockUseObservers.mockReturnValue({ data: { sessions: [makeSession()], count: 1 } })
    const { queryClient } = renderWithQuery(<ActiveObserversPanel />)
    const spy = vi.spyOn(queryClient, 'invalidateQueries')

    act(() => {
      capturedWSListener!(makeWSEvent('agent.started', {}))
    })

    expect(spy).not.toHaveBeenCalled()
  })

  it('unrelated WS event type is ignored', () => {
    mockUseObservers.mockReturnValue({ data: { sessions: [makeSession()], count: 1 } })
    const { queryClient } = renderWithQuery(<ActiveObserversPanel />)
    const spy = vi.spyOn(queryClient, 'invalidateQueries')

    act(() => {
      capturedWSListener!(makeWSEvent('workflow.updated', { kind: 'observer' }))
    })

    expect(spy).not.toHaveBeenCalled()
  })
})
