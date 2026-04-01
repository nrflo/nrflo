import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { TicketTable } from './TicketTable'
import type { PendingTicket } from '@/types/ticket'

const mockNavigate = vi.fn()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom')
  return { ...actual, useNavigate: () => mockNavigate }
})

function makeTicket(overrides: Partial<PendingTicket> = {}): PendingTicket {
  return {
    id: 'TICKET-1',
    title: 'Test ticket',
    description: null,
    status: 'open',
    priority: 2,
    issue_type: 'task',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    closed_at: null,
    created_by: 'alice',
    close_reason: null,
    is_blocked: false,
    ...overrides,
  }
}

interface TableProps {
  tickets?: PendingTicket[]
  isLoading?: boolean
  error?: Error | null
  emptyMessage?: string
  sortBy?: string
  sortOrder?: string
  onSortChange?: (col: string) => void
}

function renderTable(props: TableProps = {}) {
  const merged = {
    tickets: undefined as PendingTicket[] | undefined,
    isLoading: false,
    error: null,
    sortBy: 'updated_at',
    sortOrder: 'desc',
    onSortChange: vi.fn(),
    ...props,
  }
  return render(
    <MemoryRouter>
      <TicketTable {...merged} />
    </MemoryRouter>
  )
}

describe('TicketTable', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('non-table states', () => {
    it('shows no table while loading', () => {
      renderTable({ isLoading: true })
      expect(screen.queryByRole('table')).not.toBeInTheDocument()
    })

    it('shows error message when error is set', () => {
      renderTable({ error: new Error('fetch failed') })
      expect(screen.getByText(/Error loading tickets: fetch failed/)).toBeInTheDocument()
      expect(screen.queryByRole('table')).not.toBeInTheDocument()
    })

    it('shows default empty message for empty tickets array', () => {
      renderTable({ tickets: [] })
      expect(screen.getByText('No tickets found')).toBeInTheDocument()
    })

    it('shows custom emptyMessage', () => {
      renderTable({ tickets: [], emptyMessage: 'Nothing here yet' })
      expect(screen.getByText('Nothing here yet')).toBeInTheDocument()
    })

    it('shows empty state when tickets is undefined', () => {
      renderTable()
      expect(screen.getByText('No tickets found')).toBeInTheDocument()
    })
  })

  describe('column headers', () => {
    it('renders all column headers', () => {
      renderTable({ tickets: [makeTicket()] })
      for (const header of ['ID', 'Title', 'Type', 'Status', 'Priority', 'Created By', 'Updated', 'Progress']) {
        expect(screen.getByText(header)).toBeInTheDocument()
      }
    })

    it('Type column is the first header', () => {
      const { container } = renderTable({ tickets: [makeTicket()] })
      const headers = container.querySelectorAll('thead th')
      expect(headers[0]).toHaveTextContent('Type')
    })

    it('type icon cell is the first cell in each row', () => {
      const { container } = renderTable({ tickets: [makeTicket()] })
      const firstRow = container.querySelector('tbody tr')!
      const cells = firstRow.querySelectorAll('td')
      // First cell should contain an SVG (the IssueTypeIcon), not text like an ID
      expect(cells[0].querySelector('svg')).toBeInTheDocument()
      // Second cell should contain the ticket ID text
      expect(cells[1]).toHaveTextContent('TICKET-1')
    })
  })

  describe('row data', () => {
    it('renders ticket fields in a row', () => {
      renderTable({
        tickets: [makeTicket({ id: 'PROJ-42', title: 'My ticket', status: 'in_progress', priority: 1, created_by: 'bob' })],
      })
      expect(screen.getByText('PROJ-42')).toBeInTheDocument()
      expect(screen.getByText('My ticket')).toBeInTheDocument()
      expect(screen.getByText('in progress')).toBeInTheDocument()
      expect(screen.getByText('Critical')).toBeInTheDocument()  // priorityLabel(1) = 'Critical'
      expect(screen.getByText('bob')).toBeInTheDocument()
    })

    it('renders dash when created_by is empty string', () => {
      renderTable({ tickets: [makeTicket({ created_by: '' })] })
      expect(screen.getByText('-')).toBeInTheDocument()
    })

    it('renders multiple rows', () => {
      renderTable({
        tickets: [
          makeTicket({ id: 'T-1', title: 'First' }),
          makeTicket({ id: 'T-2', title: 'Second' }),
        ],
      })
      expect(screen.getByText('T-1')).toBeInTheDocument()
      expect(screen.getByText('T-2')).toBeInTheDocument()
    })
  })

  describe('blocked indicator', () => {
    it('shows Lock icon for blocked ticket', () => {
      const { container } = renderTable({
        tickets: [makeTicket({ is_blocked: true })],
      })
      expect(container.querySelector('[title="Blocked"]')).toBeInTheDocument()
    })

    it('does not show Lock icon for non-blocked ticket', () => {
      const { container } = renderTable({
        tickets: [makeTicket({ is_blocked: false })],
      })
      expect(container.querySelector('[title="Blocked"]')).not.toBeInTheDocument()
    })
  })

  describe('workflow progress', () => {
    it('shows X/Y progress text when total_phases > 0', () => {
      renderTable({
        tickets: [makeTicket({
          workflow_progress: { workflow_name: 'feature', current_phase: 'impl', completed_phases: 3, total_phases: 5, status: 'active' },
        })],
      })
      expect(screen.getByText('3/5')).toBeInTheDocument()
    })

    it('shows no progress when total_phases is 0', () => {
      renderTable({
        tickets: [makeTicket({
          workflow_progress: { workflow_name: 'hotfix', current_phase: 'impl', completed_phases: 0, total_phases: 0, status: 'active' },
        })],
      })
      expect(screen.queryByText(/\d+\/\d+/)).not.toBeInTheDocument()
    })

    it('shows no progress when workflow_progress is undefined', () => {
      renderTable({ tickets: [makeTicket({ workflow_progress: undefined })] })
      expect(screen.queryByText(/\d+\/\d+/)).not.toBeInTheDocument()
    })
  })

  describe('sort interaction', () => {
    it('calls onSortChange with column key when a sortable header is clicked', async () => {
      const user = userEvent.setup()
      const onSortChange = vi.fn()
      renderTable({ tickets: [makeTicket()], onSortChange })

      await user.click(screen.getByText('Priority'))

      expect(onSortChange).toHaveBeenCalledWith('priority')
    })

    it('calls onSortChange with correct key for each clickable header', async () => {
      const user = userEvent.setup()
      const onSortChange = vi.fn()
      renderTable({ tickets: [makeTicket()], onSortChange })

      await user.click(screen.getByText('ID'))
      await user.click(screen.getByText('Status'))

      expect(onSortChange).toHaveBeenCalledWith('id')
      expect(onSortChange).toHaveBeenCalledWith('status')
    })
  })

  describe('row navigation', () => {
    it('navigates to ticket detail page on row click', async () => {
      const user = userEvent.setup()
      renderTable({ tickets: [makeTicket({ id: 'PROJ-99', title: 'Click me' })] })

      await user.click(screen.getByText('Click me'))

      expect(mockNavigate).toHaveBeenCalledWith('/tickets/PROJ-99')
    })

    it('encodes special characters in ticket id for navigation', async () => {
      const user = userEvent.setup()
      renderTable({ tickets: [makeTicket({ id: 'PROJ/123', title: 'Slash ticket' })] })

      await user.click(screen.getByText('Slash ticket'))

      expect(mockNavigate).toHaveBeenCalledWith('/tickets/PROJ%2F123')
    })
  })
})
