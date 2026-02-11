import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { EditTicketPage } from './EditTicketPage'
import * as ticketsApi from '@/api/tickets'
import type { TicketWithDeps } from '@/types/ticket'

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
    getTicket: vi.fn(),
    updateTicket: vi.fn(),
  }
})

const sampleTicket: TicketWithDeps = {
  id: 'TICKET-1',
  title: 'Test ticket title',
  description: 'Test description',
  status: 'open',
  priority: 3,
  issue_type: 'bug',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  closed_at: null,
  created_by: 'tester',
  close_reason: null,
  blockers: [],
  blocks: [],
}

function renderPage(ticketId = 'TICKET-1') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/tickets/${encodeURIComponent(ticketId)}/edit`]}>
        <Routes>
          <Route path="/tickets/:id/edit" element={<EditTicketPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('EditTicketPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading spinner while ticket is loading', () => {
    vi.mocked(ticketsApi.getTicket).mockReturnValue(new Promise(() => {}))
    renderPage()
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('shows error when ticket fails to load', async () => {
    vi.mocked(ticketsApi.getTicket).mockRejectedValue(new Error('Not found'))
    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/Error: Not found/)).toBeInTheDocument()
    })
    expect(screen.getByRole('link', { name: /back to tickets/i })).toBeInTheDocument()
  })

  it('renders form with ticket data pre-filled', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    renderPage()

    await waitFor(() => {
      expect(screen.getByDisplayValue('Test ticket title')).toBeInTheDocument()
    })
    expect(screen.getByDisplayValue('Test description')).toBeInTheDocument()
    expect(screen.getByLabelText('Ticket ID')).toHaveValue('TICKET-1')
    expect(screen.getByLabelText('Ticket ID')).toBeDisabled()
  })

  it('renders form with null description as empty', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue({
      ...sampleTicket,
      description: null,
    })
    renderPage()

    await waitFor(() => {
      expect(screen.getByDisplayValue('Test ticket title')).toBeInTheDocument()
    })
    expect(screen.getByLabelText('Description')).toHaveValue('')
  })

  it('submits update with correct fields and navigates to ticket detail', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.updateTicket).mockResolvedValue({
      ...sampleTicket,
      title: 'Updated title',
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByDisplayValue('Test ticket title')).toBeInTheDocument()
    })

    const titleInput = screen.getByLabelText('Title')
    await user.clear(titleInput)
    await user.type(titleInput, 'Updated title')
    await user.click(screen.getByRole('button', { name: /update ticket/i }))

    await waitFor(() => {
      expect(ticketsApi.updateTicket).toHaveBeenCalledTimes(1)
    })
    const [id, data] = vi.mocked(ticketsApi.updateTicket).mock.calls[0]
    expect(id).toBe('TICKET-1')
    expect(data.title).toBe('Updated title')
    expect(data.description).toBe('Test description')
    expect(data.priority).toBe(3)
    expect(data.issue_type).toBe('bug')
    // Should NOT include id or created_by in the update payload
    expect(data).not.toHaveProperty('id')
    expect(data).not.toHaveProperty('created_by')

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/tickets/TICKET-1')
    })
  })

  it('shows error when update mutation fails', async () => {
    const user = userEvent.setup()
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    vi.mocked(ticketsApi.updateTicket).mockRejectedValue(new Error('Update failed'))

    renderPage()

    await waitFor(() => {
      expect(screen.getByDisplayValue('Test ticket title')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: /update ticket/i }))

    await waitFor(() => {
      expect(screen.getByText(/Update failed/)).toBeInTheDocument()
    })
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('back button links to ticket detail page, not ticket list', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    renderPage()

    await waitFor(() => {
      expect(screen.getByDisplayValue('Test ticket title')).toBeInTheDocument()
    })

    const backLink = screen.getByRole('link', { name: '' })
    expect(backLink).toHaveAttribute('href', '/tickets/TICKET-1')
  })

  it('properly decodes URL-encoded ticket IDs', async () => {
    const encodedTicket: TicketWithDeps = {
      ...sampleTicket,
      id: 'nrworkflow-05a95b',
    }
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(encodedTicket)

    renderPage('nrworkflow-05a95b')

    await waitFor(() => {
      expect(ticketsApi.getTicket).toHaveBeenCalledWith('nrworkflow-05a95b')
    })
  })

  it('handles ticket IDs with special characters', async () => {
    const user = userEvent.setup()
    const specialTicket: TicketWithDeps = {
      ...sampleTicket,
      id: 'PROJECT/TICKET 42',
    }
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(specialTicket)
    vi.mocked(ticketsApi.updateTicket).mockResolvedValue({
      ...sampleTicket,
      id: 'PROJECT/TICKET 42',
    })

    renderPage('PROJECT/TICKET 42')

    await waitFor(() => {
      expect(screen.getByLabelText('Ticket ID')).toHaveValue('PROJECT/TICKET 42')
    })

    await user.click(screen.getByRole('button', { name: /update ticket/i }))

    await waitFor(() => {
      expect(ticketsApi.updateTicket).toHaveBeenCalledTimes(1)
    })
    expect(vi.mocked(ticketsApi.updateTicket).mock.calls[0][0]).toBe('PROJECT/TICKET 42')

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/tickets/PROJECT%2FTICKET%2042')
    })
  })

  it('displays the ticket ID in the card header', async () => {
    vi.mocked(ticketsApi.getTicket).mockResolvedValue(sampleTicket)
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('TICKET-1')).toBeInTheDocument()
    })
    expect(screen.getByText('Edit Ticket')).toBeInTheDocument()
  })
})
