import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { InteractiveSessionsTray } from './InteractiveSessionsTray'
import type { InteractiveSession } from '@/stores/interactiveSessionsStore'
import type { WSEvent } from '@/hooks/useWebSocket'
import type { UserEvent } from '@testing-library/user-event'

// ---- store mock ----
const mockRemove = vi.fn()
const mockSetActive = vi.fn()
const mockToggleMinimized = vi.fn()

vi.mock('@/stores/interactiveSessionsStore', () => ({
  useInteractiveSessionsStore: vi.fn(),
}))

import { useInteractiveSessionsStore } from '@/stores/interactiveSessionsStore'

// ---- WS context mock ----
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

// ---- mutation hooks mock ----
const mockExitTicketMutate = vi.fn()
const mockExitProjectMutate = vi.fn()
const mockKillTicketMutate = vi.fn()
const mockKillProjectMutate = vi.fn()

vi.mock('@/hooks/useTickets', () => ({
  useExitInteractive: () => ({ mutate: mockExitTicketMutate, isPending: false }),
  useExitInteractiveProject: () => ({ mutate: mockExitProjectMutate, isPending: false }),
  useKillInteractive: () => ({ mutate: mockKillTicketMutate, isPending: false }),
  useKillInteractiveProject: () => ({ mutate: mockKillProjectMutate, isPending: false }),
}))

// ---- InteractiveSessionPanel mock (avoid lazy XTerminal) ----
vi.mock('./InteractiveSessionPanel', () => ({
  InteractiveSessionPanel: ({ sessionId, isActive }: { sessionId: string; isActive: boolean }) =>
    isActive ? <div data-testid="session-panel" data-session={sessionId} /> : null,
}))

// ---- helpers ----
const makeSession = (overrides: Partial<InteractiveSession> = {}): InteractiveSession => ({
  sessionId: 'sess-1',
  agentType: 'setup-analyzer',
  scope: { type: 'ticket', ticketId: 'T-1' },
  workflow: 'feature',
  startedAt: 0,
  ...overrides,
})

type StoreOverrides = {
  sessions?: InteractiveSession[]
  activeId?: string
  minimized?: boolean
}

function setStore({ sessions = [], activeId = 'sess-1', minimized = false }: StoreOverrides = {}) {
  vi.mocked(useInteractiveSessionsStore).mockImplementation((selector: any) =>
    selector({ sessions, activeId, minimized, setActive: mockSetActive, toggleMinimized: mockToggleMinimized, remove: mockRemove })
  )
}

// The kill button (X icon) is always the second-to-last button in the tray header.
// Order: [session tabs...] [Exit Session] [X/kill] [Minimize]
async function clickKillButton(user: UserEvent) {
  const buttons = screen.getAllByRole('button')
  await user.click(buttons[buttons.length - 2])
}

describe('InteractiveSessionsTray', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    capturedWSListener = null
  })

  it('renders nothing when sessions list is empty', () => {
    setStore({ sessions: [] })
    const { container } = render(<InteractiveSessionsTray />)
    expect(container.firstChild).toBeNull()
  })

  it('renders tray when sessions exist', () => {
    setStore({ sessions: [makeSession()] })
    render(<InteractiveSessionsTray />)
    expect(screen.getByTestId('session-panel')).toBeInTheDocument()
  })

  it('shows session tab label with agent type and short id', () => {
    setStore({ sessions: [makeSession({ sessionId: 'abcdef1234' })] })
    render(<InteractiveSessionsTray />)
    expect(screen.getByText('setup-analyzer (abcdef)')).toBeInTheDocument()
  })

  it('shows a tab for each session', () => {
    setStore({
      sessions: [
        makeSession({ sessionId: 'aaa111', agentType: 'implementor' }),
        makeSession({ sessionId: 'bbb222', agentType: 'setup-analyzer' }),
      ],
      activeId: 'bbb222',
    })
    render(<InteractiveSessionsTray />)
    expect(screen.getByText('implementor (aaa111)')).toBeInTheDocument()
    expect(screen.getByText('setup-analyzer (bbb222)')).toBeInTheDocument()
  })

  it('hides session panels when minimized', () => {
    setStore({ sessions: [makeSession()], minimized: true })
    render(<InteractiveSessionsTray />)
    expect(screen.queryByTestId('session-panel')).not.toBeInTheDocument()
  })

  it('calls toggleMinimized when minimize button clicked', async () => {
    const user = userEvent.setup()
    setStore({ sessions: [makeSession()], minimized: false })
    render(<InteractiveSessionsTray />)

    await user.click(screen.getByTitle('Minimize'))
    expect(mockToggleMinimized).toHaveBeenCalledOnce()
  })

  it('shows plan-mode footer only when agentType is planner', () => {
    setStore({ sessions: [makeSession({ agentType: 'planner' })] })
    render(<InteractiveSessionsTray />)
    expect(screen.getByText(/plan file will be used as instructions/i)).toBeInTheDocument()
  })

  it('does not show plan-mode footer for non-planner agents', () => {
    setStore({ sessions: [makeSession({ agentType: 'setup-analyzer' })] })
    render(<InteractiveSessionsTray />)
    expect(screen.queryByText(/plan file will be used as instructions/i)).not.toBeInTheDocument()
  })

  describe('Exit Session', () => {
    it('calls exitInteractive mutation for ticket-scoped session', async () => {
      const user = userEvent.setup()
      setStore({ sessions: [makeSession({ scope: { type: 'ticket', ticketId: 'T-42' } })] })
      render(<InteractiveSessionsTray />)

      await user.click(screen.getByRole('button', { name: /exit session/i }))
      expect(mockExitTicketMutate).toHaveBeenCalledWith(
        expect.objectContaining({ ticketId: 'T-42' }),
        expect.objectContaining({ onSuccess: expect.any(Function) })
      )
    })

    it('calls exitInteractiveProject mutation for project-scoped session', async () => {
      const user = userEvent.setup()
      setStore({ sessions: [makeSession({ scope: { type: 'project', projectId: 'proj-1' } })] })
      render(<InteractiveSessionsTray />)

      await user.click(screen.getByRole('button', { name: /exit session/i }))
      expect(mockExitProjectMutate).toHaveBeenCalledWith(
        expect.objectContaining({ projectId: 'proj-1' }),
        expect.objectContaining({ onSuccess: expect.any(Function) })
      )
    })

    it('calls remove via onSuccess callback', async () => {
      const user = userEvent.setup()
      mockExitTicketMutate.mockImplementation((_params: unknown, opts: any) => opts?.onSuccess?.())
      setStore({ sessions: [makeSession()] })
      render(<InteractiveSessionsTray />)

      await user.click(screen.getByRole('button', { name: /exit session/i }))
      expect(mockRemove).toHaveBeenCalledWith('sess-1')
    })
  })

  describe('Close (kill) Session', () => {
    it('opens ConfirmDialog when kill button is clicked', async () => {
      const user = userEvent.setup()
      setStore({ sessions: [makeSession()] })
      render(<InteractiveSessionsTray />)

      await clickKillButton(user)
      // Dialog body text is unique to the kill confirm dialog
      await screen.findByText(/force-close this interactive session/i)
    })

    it('calls killInteractive for ticket-scoped session on confirm', async () => {
      const user = userEvent.setup()
      setStore({ sessions: [makeSession({ scope: { type: 'ticket', ticketId: 'T-5' } })] })
      render(<InteractiveSessionsTray />)

      await clickKillButton(user)
      await user.click(await screen.findByRole('button', { name: /close session/i }))

      expect(mockKillTicketMutate).toHaveBeenCalledWith(
        expect.objectContaining({ ticketId: 'T-5' }),
        expect.objectContaining({ onSuccess: expect.any(Function) })
      )
    })

    it('calls killInteractiveProject for project-scoped session on confirm', async () => {
      const user = userEvent.setup()
      setStore({ sessions: [makeSession({ scope: { type: 'project', projectId: 'proj-7' } })] })
      render(<InteractiveSessionsTray />)

      await clickKillButton(user)
      await user.click(await screen.findByRole('button', { name: /close session/i }))

      expect(mockKillProjectMutate).toHaveBeenCalledWith(
        expect.objectContaining({ projectId: 'proj-7' }),
        expect.objectContaining({ onSuccess: expect.any(Function) })
      )
    })

    it('calls remove via onSuccess callback', async () => {
      const user = userEvent.setup()
      mockKillTicketMutate.mockImplementation((_params: unknown, opts: any) => opts?.onSuccess?.())
      setStore({ sessions: [makeSession()] })
      render(<InteractiveSessionsTray />)

      await clickKillButton(user)
      await user.click(await screen.findByRole('button', { name: /close session/i }))

      expect(mockRemove).toHaveBeenCalledWith('sess-1')
    })
  })

  describe('WS event handling', () => {
    it('registers a WS event listener on mount', () => {
      setStore({ sessions: [makeSession()] })
      render(<InteractiveSessionsTray />)
      expect(mockAddEventListener).toHaveBeenCalledOnce()
    })

    it('removes WS listener on unmount', () => {
      setStore({ sessions: [makeSession()] })
      const { unmount } = render(<InteractiveSessionsTray />)
      unmount()
      expect(mockRemoveEventListener).toHaveBeenCalledOnce()
    })

    it('calls remove on agent.killed event with matching session_id', () => {
      setStore({ sessions: [makeSession()] })
      render(<InteractiveSessionsTray />)

      capturedWSListener!({
        type: 'agent.killed',
        data: { session_id: 'sess-1' },
        project_id: 'p',
        ticket_id: 't',
        timestamp: '',
      })
      expect(mockRemove).toHaveBeenCalledWith('sess-1')
    })

    it('calls remove on agent.completed event with matching session_id', () => {
      setStore({ sessions: [makeSession()] })
      render(<InteractiveSessionsTray />)

      capturedWSListener!({
        type: 'agent.completed',
        data: { session_id: 'sess-1' },
        project_id: 'p',
        ticket_id: 't',
        timestamp: '',
      })
      expect(mockRemove).toHaveBeenCalledWith('sess-1')
    })

    it('does not call remove when event has no session_id', () => {
      setStore({ sessions: [makeSession()] })
      render(<InteractiveSessionsTray />)

      capturedWSListener!({
        type: 'agent.killed',
        data: {},
        project_id: 'p',
        ticket_id: 't',
        timestamp: '',
      })
      expect(mockRemove).not.toHaveBeenCalled()
    })

    it('ignores irrelevant WS event types', () => {
      setStore({ sessions: [makeSession()] })
      render(<InteractiveSessionsTray />)

      capturedWSListener!({
        type: 'agent.started',
        data: { session_id: 'sess-1' },
        project_id: 'p',
        ticket_id: 't',
        timestamp: '',
      })
      expect(mockRemove).not.toHaveBeenCalled()
    })
  })
})
