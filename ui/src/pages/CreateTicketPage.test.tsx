import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { CreateTicketPage } from './CreateTicketPage'
import * as ticketsApi from '@/api/tickets'

const mockNavigate = vi.fn()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true }),
}))

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return {
    ...actual,
    createTicket: vi.fn(),
  }
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <CreateTicketPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('CreateTicketPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('converts empty ID to undefined and navigates to server-returned ID', async () => {
    const user = userEvent.setup()
    const serverTicket = {
      id: 'auto-gen-123',
      title: 'Test',
      description: null,
      status: 'open' as const,
      priority: 2,
      issue_type: 'task' as const,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      closed_at: null,
      created_by: 'ui',
      close_reason: null,
    }
    vi.mocked(ticketsApi.createTicket).mockResolvedValue(serverTicket)

    renderPage()

    // Leave ID empty, fill required fields
    await user.type(screen.getByLabelText('Title'), 'Test')
    await user.click(screen.getByRole('button', { name: /create ticket/i }))

    await waitFor(() => {
      expect(ticketsApi.createTicket).toHaveBeenCalledTimes(1)
    })
    const request = vi.mocked(ticketsApi.createTicket).mock.calls[0][0]
    expect(request.id).toBeUndefined()

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/tickets/auto-gen-123')
    })
  })

  it('passes custom ID through and navigates to server-returned ID', async () => {
    const user = userEvent.setup()
    const serverTicket = {
      id: 'CUSTOM-42',
      title: 'Custom ticket',
      description: null,
      status: 'open' as const,
      priority: 2,
      issue_type: 'task' as const,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      closed_at: null,
      created_by: 'ui',
      close_reason: null,
    }
    vi.mocked(ticketsApi.createTicket).mockResolvedValue(serverTicket)

    renderPage()

    await user.type(screen.getByLabelText('Ticket ID'), 'CUSTOM-42')
    await user.type(screen.getByLabelText('Title'), 'Custom ticket')
    await user.click(screen.getByRole('button', { name: /create ticket/i }))

    await waitFor(() => {
      expect(ticketsApi.createTicket).toHaveBeenCalledTimes(1)
    })
    const request = vi.mocked(ticketsApi.createTicket).mock.calls[0][0]
    expect(request.id).toBe('CUSTOM-42')

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/tickets/CUSTOM-42')
    })
  })

  it('shows error message when API call fails', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.createTicket).mockRejectedValue(new Error('Server error'))

    renderPage()

    await user.type(screen.getByLabelText('Title'), 'Failing ticket')
    await user.click(screen.getByRole('button', { name: /create ticket/i }))

    await waitFor(() => {
      expect(screen.getByText(/server error/i)).toBeInTheDocument()
    })
    expect(mockNavigate).not.toHaveBeenCalled()
  })
})
