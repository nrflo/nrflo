import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { LogsSessionsTab } from './LogsSessionsTab'
import { useSessions } from '@/hooks/useSessions'

vi.mock('@/hooks/useSessions', async () => {
  const actual = await vi.importActual('@/hooks/useSessions')
  return { ...actual, useSessions: vi.fn() }
})
vi.mock('@/components/sessions/SessionDetail', () => ({
  SessionDetail: ({ sessionId }: { sessionId: string }) => <div>detail-for-{sessionId}</div>,
}))

const emptyList = { sessions: [] }

function renderTab(initialEntry = '/logs?tab=sessions') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/logs" element={<LogsSessionsTab />} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.mocked(useSessions).mockReset()
  vi.mocked(useSessions).mockReturnValue({ data: emptyList, isLoading: false } as any)
})

describe('LogsSessionsTab', () => {
  it('shows a loading state while sessions are fetching', () => {
    vi.mocked(useSessions).mockReturnValue({ data: undefined, isLoading: true } as any)
    renderTab()
    expect(screen.getByText('Loading...')).toBeInTheDocument()
  })

  it('defaults to the project scope', () => {
    renderTab()
    expect(useSessions).toHaveBeenCalledWith('project', { limit: 100 })
  })

  it('switches to the global scope on click', async () => {
    const user = userEvent.setup()
    renderTab()
    await user.click(screen.getByRole('button', { name: 'All projects' }))
    expect(useSessions).toHaveBeenLastCalledWith('global', { limit: 100 })
  })

  it('renders no session detail pane without ?sid=', () => {
    renderTab('/logs?tab=sessions')
    expect(screen.queryByText(/^detail-for-/)).not.toBeInTheDocument()
  })

  it('renders the session detail pane for ?sid=', () => {
    renderTab('/logs?tab=sessions&sid=session-1')
    expect(screen.getByText('detail-for-session-1')).toBeInTheDocument()
  })
})
