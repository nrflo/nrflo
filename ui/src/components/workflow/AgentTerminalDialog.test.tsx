import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentTerminalDialog } from './AgentTerminalDialog'

// XTerminal lazy-loads xterm.js and opens a WebSocket — mock it entirely
vi.mock('./XTerminal', () => ({
  XTerminal: ({ sessionId, onExit }: { sessionId: string; onExit: () => void }) => (
    <div data-testid="xterm-terminal" data-session-id={sessionId}>
      <button onClick={onExit}>Simulate Exit</button>
    </div>
  ),
}))

function renderDialog(overrides: {
  open?: boolean
  onClose?: () => void
  onExitSession?: () => void
  exitPending?: boolean
  sessionId?: string
  agentType?: string
} = {}) {
  const props = {
    open: true,
    onClose: vi.fn(),
    onExitSession: vi.fn(),
    exitPending: false,
    sessionId: 'sess-abc-123',
    agentType: 'implementor',
    ...overrides,
  }
  return { ...render(<AgentTerminalDialog {...props} />), props }
}

describe('AgentTerminalDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('rendering', () => {
    it('renders dialog header with agent type when open', () => {
      renderDialog({ agentType: 'test-runner' })
      expect(screen.getByText('Interactive Control')).toBeInTheDocument()
      expect(screen.getByText('— test-runner')).toBeInTheDocument()
    })

    it('renders XTerminal with correct sessionId when open', async () => {
      renderDialog({ sessionId: 'my-session-id' })
      // XTerminal is lazy-loaded — wait for Suspense to resolve
      const terminal = await screen.findByTestId('xterm-terminal')
      expect(terminal).toBeInTheDocument()
      expect(terminal.getAttribute('data-session-id')).toBe('my-session-id')
    })

    it('does not render XTerminal when dialog is closed', () => {
      renderDialog({ open: false })
      expect(screen.queryByTestId('xterm-terminal')).not.toBeInTheDocument()
    })

    it('renders Exit Session button', () => {
      renderDialog()
      expect(screen.getByRole('button', { name: /exit session/i })).toBeInTheDocument()
    })
  })

  describe('Exit Session button', () => {
    it('calls onExitSession when Exit Session is clicked', async () => {
      const user = userEvent.setup()
      const onExitSession = vi.fn()
      renderDialog({ onExitSession })

      await user.click(screen.getByRole('button', { name: /exit session/i }))
      expect(onExitSession).toHaveBeenCalledTimes(1)
    })

    it('disables Exit Session button when exitPending is true', () => {
      renderDialog({ exitPending: true })
      expect(screen.getByRole('button', { name: /exit session/i })).toBeDisabled()
    })

    it('enables Exit Session button when exitPending is false', () => {
      renderDialog({ exitPending: false })
      expect(screen.getByRole('button', { name: /exit session/i })).not.toBeDisabled()
    })
  })

  describe('plan mode hint', () => {
    const HINT_TEXT = 'On exit, the plan file will be used as instructions for workflow agents. Use \'/plan\' to show the plan.'

    it('shows hint text when agentType is planner', () => {
      renderDialog({ agentType: 'planner' })
      expect(screen.getByText(HINT_TEXT)).toBeInTheDocument()
    })

    it('does not show hint text when agentType is agent', () => {
      renderDialog({ agentType: 'agent' })
      expect(screen.queryByText(HINT_TEXT)).not.toBeInTheDocument()
    })

    it('does not show hint text for workflow-name agentType', () => {
      renderDialog({ agentType: 'feature' })
      expect(screen.queryByText(HINT_TEXT)).not.toBeInTheDocument()
    })

    it('Exit Session button is present regardless of agentType', () => {
      renderDialog({ agentType: 'planner' })
      expect(screen.getByRole('button', { name: /exit session/i })).toBeInTheDocument()
    })
  })

  describe('non-dismissable behavior', () => {
    it('renders a header close button (X icon)', () => {
      const onClose = vi.fn()
      renderDialog({ onClose })
      // DialogHeader renders an X button — find button that is not "Exit Session"
      const allButtons = screen.getAllByRole('button')
      const closeButton = allButtons.find((b) => !b.textContent?.includes('Exit Session'))
      expect(closeButton).toBeInTheDocument()
    })

    it('calls onClose when header X button is clicked', async () => {
      const user = userEvent.setup()
      const onClose = vi.fn()
      renderDialog({ onClose })
      const allButtons = screen.getAllByRole('button')
      const closeButton = allButtons.find((b) => !b.textContent?.includes('Exit Session'))!
      await user.click(closeButton)
      expect(onClose).toHaveBeenCalledTimes(1)
    })
  })
})
