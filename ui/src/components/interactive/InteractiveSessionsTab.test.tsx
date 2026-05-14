import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { InteractiveSessionsTab } from './InteractiveSessionsTab'
import type { InteractiveSession } from '@/stores/interactiveSessionsStore'

const mockToggleMinimized = vi.fn()

vi.mock('@/stores/interactiveSessionsStore', () => ({
  useInteractiveSessionsStore: vi.fn(),
}))

import { useInteractiveSessionsStore } from '@/stores/interactiveSessionsStore'

function setStore(sessions: InteractiveSession[] = []) {
  vi.mocked(useInteractiveSessionsStore).mockImplementation((selector: any) =>
    selector({ sessions, toggleMinimized: mockToggleMinimized })
  )
}

const makeSession = (id: string): InteractiveSession => ({
  sessionId: id,
  agentType: 'setup-analyzer',
  scope: { type: 'ticket', ticketId: 'T-1' },
  workflow: 'feature',
  startedAt: 0,
})

describe('InteractiveSessionsTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when there are no sessions', () => {
    setStore([])
    const { container } = render(<InteractiveSessionsTab />)
    expect(container.firstChild).toBeNull()
  })

  it('shows session count with a single session', () => {
    setStore([makeSession('a')])
    render(<InteractiveSessionsTab />)
    expect(screen.getByText('Sessions (1)')).toBeInTheDocument()
  })

  it('shows correct count for multiple sessions', () => {
    setStore([makeSession('a'), makeSession('b'), makeSession('c')])
    render(<InteractiveSessionsTab />)
    expect(screen.getByText('Sessions (3)')).toBeInTheDocument()
  })

  it('calls toggleMinimized when clicked', async () => {
    const user = userEvent.setup()
    setStore([makeSession('a')])
    render(<InteractiveSessionsTab />)

    await user.click(screen.getByTitle('Interactive Sessions'))
    expect(mockToggleMinimized).toHaveBeenCalledOnce()
  })
})
