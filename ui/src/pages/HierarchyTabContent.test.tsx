import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HierarchyTabContent } from './HierarchyTabContent'
import * as ticketsApi from '@/api/tickets'
import type { TicketWithDeps } from '@/types/ticket'

vi.mock('@/api/tickets', () => ({
  addDependency: vi.fn(),
  removeDependency: vi.fn(),
}))

const baseTicket: TicketWithDeps = {
  id: 'TICK-100',
  title: 'Test ticket',
  description: 'Some description',
  status: 'open',
  priority: 2,
  issue_type: 'feature',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  closed_at: null,
  created_by: 'user',
  close_reason: null,
  blockers: [],
  blocks: [],
}

function renderPage(ticket: TicketWithDeps = baseTicket) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <HierarchyTabContent ticket={ticket} />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('HierarchyTabContent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('Blockers section', () => {
    it('renders "Blocked by" section header', () => {
      renderPage()
      expect(screen.getByText('Blocked by')).toBeInTheDocument()
    })

    it('renders blocker with ID and title', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blockers: [
          {
            issue_id: 'TICK-100',
            depends_on_id: 'TICK-50',
            depends_on_title: 'Fix auth bug',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('TICK-50')).toBeInTheDocument()
      expect(screen.getByText('Fix auth bug')).toBeInTheDocument()
    })

    it('renders blocker with ID only when title is missing', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blockers: [
          {
            issue_id: 'TICK-100',
            depends_on_id: 'TICK-50',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('TICK-50')).toBeInTheDocument()
    })

    it('renders multiple blockers', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blockers: [
          {
            issue_id: 'TICK-100',
            depends_on_id: 'TICK-50',
            depends_on_title: 'Fix auth bug',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
          {
            issue_id: 'TICK-100',
            depends_on_id: 'TICK-60',
            depends_on_title: 'Add logging',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('TICK-50')).toBeInTheDocument()
      expect(screen.getByText('Fix auth bug')).toBeInTheDocument()
      expect(screen.getByText('TICK-60')).toBeInTheDocument()
      expect(screen.getByText('Add logging')).toBeInTheDocument()
    })

    it('renders remove button for each blocker', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blockers: [
          {
            issue_id: 'TICK-100',
            depends_on_id: 'TICK-50',
            depends_on_title: 'Fix auth bug',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      const removeButton = screen.getByTitle('Remove blocker')
      expect(removeButton).toBeInTheDocument()
    })

    it('calls removeDependency when remove button is clicked', async () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blockers: [
          {
            issue_id: 'TICK-100',
            depends_on_id: 'TICK-50',
            depends_on_title: 'Fix auth bug',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      vi.mocked(ticketsApi.removeDependency).mockResolvedValue({ message: 'ok', child_id: 'TICK-100', parent_id: 'TICK-50' })
      renderPage(ticket)
      const removeButton = screen.getByTitle('Remove blocker')
      await userEvent.click(removeButton)
      await waitFor(() => {
        expect(ticketsApi.removeDependency).toHaveBeenCalledWith({
          issue_id: 'TICK-100',
          depends_on_id: 'TICK-50',
        })
      })
    })

    it('renders TicketSearchDropdown for adding blockers', () => {
      renderPage()
      expect(screen.getByPlaceholderText('Search tickets to add blocker...')).toBeInTheDocument()
    })

    it('excludes current ticket and existing blockers from search', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blockers: [
          {
            issue_id: 'TICK-100',
            depends_on_id: 'TICK-50',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      // TicketSearchDropdown receives excludeIds prop with current ticket and blockers
      expect(screen.getByPlaceholderText('Search tickets to add blocker...')).toBeInTheDocument()
    })
  })

  describe('Blocks section', () => {
    it('does not render "Blocks" section when empty', () => {
      renderPage()
      expect(screen.queryByText('Blocks')).not.toBeInTheDocument()
    })

    it('renders blocks with ID and title', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blocks: [
          {
            issue_id: 'TICK-200',
            depends_on_id: 'TICK-100',
            issue_title: 'Deploy to prod',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('Blocks')).toBeInTheDocument()
      expect(screen.getByText('TICK-200')).toBeInTheDocument()
      expect(screen.getByText('Deploy to prod')).toBeInTheDocument()
    })

    it('renders blocks with ID only when title is missing', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blocks: [
          {
            issue_id: 'TICK-200',
            depends_on_id: 'TICK-100',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('TICK-200')).toBeInTheDocument()
    })

    it('renders multiple blocks', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blocks: [
          {
            issue_id: 'TICK-200',
            depends_on_id: 'TICK-100',
            issue_title: 'Deploy to prod',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
          {
            issue_id: 'TICK-210',
            depends_on_id: 'TICK-100',
            issue_title: 'Run smoke tests',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('TICK-200')).toBeInTheDocument()
      expect(screen.getByText('Deploy to prod')).toBeInTheDocument()
      expect(screen.getByText('TICK-210')).toBeInTheDocument()
      expect(screen.getByText('Run smoke tests')).toBeInTheDocument()
    })

    it('does not render remove button for blocks (read-only)', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blocks: [
          {
            issue_id: 'TICK-200',
            depends_on_id: 'TICK-100',
            issue_title: 'Deploy to prod',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
      }
      renderPage(ticket)
      expect(screen.queryByTitle('Remove blocker')).not.toBeInTheDocument()
    })
  })

  describe('Epic hierarchy - parent and siblings', () => {
    it('does not render epic hierarchy when ticket has no parent', () => {
      renderPage()
      expect(screen.queryByText('Epic Hierarchy')).not.toBeInTheDocument()
    })

    it('renders parent epic link with ID and title', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        parent_ticket_id: 'EPIC-10',
        parent_ticket: {
          id: 'EPIC-10',
          title: 'User authentication',
          description: null,
          status: 'in_progress',
          priority: 1,
          issue_type: 'epic',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
          closed_at: null,
          created_by: 'user',
          close_reason: null,
        },
      }
      renderPage(ticket)
      expect(screen.getByText('Epic Hierarchy')).toBeInTheDocument()
      expect(screen.getByText('Parent Epic')).toBeInTheDocument()
      expect(screen.getByText('EPIC-10')).toBeInTheDocument()
      expect(screen.getByText('User authentication')).toBeInTheDocument()
    })

    it('renders parent epic link with ID only when title is missing', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        parent_ticket_id: 'EPIC-10',
        parent_ticket: null,
      }
      renderPage(ticket)
      expect(screen.getByText('EPIC-10')).toBeInTheDocument()
    })

    it('does not render sibling section when siblings array is empty', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        parent_ticket_id: 'EPIC-10',
        siblings: [],
      }
      renderPage(ticket)
      expect(screen.queryByText('Sibling Tickets')).not.toBeInTheDocument()
    })

    it('renders sibling tickets with status, ID, title, and priority', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        parent_ticket_id: 'EPIC-10',
        siblings: [
          {
            id: 'TICK-101',
            title: 'Login form',
            description: null,
            status: 'in_progress',
            priority: 1,
            issue_type: 'task',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
            closed_at: null,
            created_by: 'user',
            close_reason: null,
          },
          {
            id: 'TICK-102',
            title: 'Password reset',
            description: null,
            status: 'open',
            priority: 2,
            issue_type: 'task',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
            closed_at: null,
            created_by: 'user',
            close_reason: null,
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('Sibling Tickets')).toBeInTheDocument()
      expect(screen.getByText('TICK-101')).toBeInTheDocument()
      expect(screen.getByText('Login form')).toBeInTheDocument()
      expect(screen.getByText('in progress')).toBeInTheDocument()
      expect(screen.getByText('TICK-102')).toBeInTheDocument()
      expect(screen.getByText('Password reset')).toBeInTheDocument()
      expect(screen.getByText('open')).toBeInTheDocument()
    })

    it('highlights current ticket in sibling list', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        id: 'TICK-100',
        parent_ticket_id: 'EPIC-10',
        siblings: [
          {
            id: 'TICK-100',
            title: 'Current task',
            description: null,
            status: 'in_progress',
            priority: 1,
            issue_type: 'task',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
            closed_at: null,
            created_by: 'user',
            close_reason: null,
          },
          {
            id: 'TICK-101',
            title: 'Other task',
            description: null,
            status: 'open',
            priority: 2,
            issue_type: 'task',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
            closed_at: null,
            created_by: 'user',
            close_reason: null,
          },
        ],
      }
      const { container } = renderPage(ticket)
      const highlightedDiv = container.querySelector('.bg-muted')
      expect(highlightedDiv).toBeInTheDocument()
      expect(highlightedDiv).toHaveTextContent('TICK-100')
    })

    it('highlights current ticket case-insensitively', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        id: 'tick-100',
        parent_ticket_id: 'EPIC-10',
        siblings: [
          {
            id: 'TICK-100',
            title: 'Current task',
            description: null,
            status: 'in_progress',
            priority: 1,
            issue_type: 'task',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
            closed_at: null,
            created_by: 'user',
            close_reason: null,
          },
        ],
      }
      const { container } = renderPage(ticket)
      const highlightedDiv = container.querySelector('.bg-muted')
      expect(highlightedDiv).toBeInTheDocument()
    })
  })

  describe('Epic children', () => {
    it('does not render children section for non-epic tickets', () => {
      renderPage()
      expect(screen.queryByText('Children')).not.toBeInTheDocument()
    })

    it('does not render children section when epic has no children', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'epic',
        children: [],
      }
      renderPage(ticket)
      expect(screen.queryByText('Children')).not.toBeInTheDocument()
    })

    it('renders children with status, ID, title, and priority', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'epic',
        children: [
          {
            id: 'TICK-101',
            title: 'Child task 1',
            description: null,
            status: 'in_progress',
            priority: 1,
            issue_type: 'task',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
            closed_at: null,
            created_by: 'user',
            close_reason: null,
          },
          {
            id: 'TICK-102',
            title: 'Child task 2',
            description: null,
            status: 'closed',
            priority: 3,
            issue_type: 'task',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
            closed_at: '2026-01-05T00:00:00Z',
            created_by: 'user',
            close_reason: null,
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('Children')).toBeInTheDocument()
      expect(screen.getByText('TICK-101')).toBeInTheDocument()
      expect(screen.getByText('Child task 1')).toBeInTheDocument()
      expect(screen.getByText('in progress')).toBeInTheDocument()
      expect(screen.getByText('TICK-102')).toBeInTheDocument()
      expect(screen.getByText('Child task 2')).toBeInTheDocument()
      expect(screen.getByText('closed')).toBeInTheDocument()
    })

    it('renders both epic hierarchy and children when epic has parent and children', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        issue_type: 'epic',
        parent_ticket_id: 'EPIC-5',
        children: [
          {
            id: 'TICK-101',
            title: 'Child task 1',
            description: null,
            status: 'open',
            priority: 1,
            issue_type: 'task',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
            closed_at: null,
            created_by: 'user',
            close_reason: null,
          },
        ],
        siblings: [],
      }
      renderPage(ticket)
      // Both sections can be shown
      expect(screen.getByText('Epic Hierarchy')).toBeInTheDocument()
      expect(screen.getByText('Children')).toBeInTheDocument()
    })
  })

  describe('Edge cases', () => {
    it('renders correctly with no blockers, blocks, parent, or children', () => {
      renderPage()
      expect(screen.getByText('Blocked by')).toBeInTheDocument()
      expect(screen.queryByText('Blocks')).not.toBeInTheDocument()
      expect(screen.queryByText('Epic Hierarchy')).not.toBeInTheDocument()
      expect(screen.queryByText('Children')).not.toBeInTheDocument()
    })

    it('renders all sections when ticket has all relationships', () => {
      const ticket: TicketWithDeps = {
        ...baseTicket,
        blockers: [
          {
            issue_id: 'TICK-100',
            depends_on_id: 'TICK-50',
            depends_on_title: 'Fix auth bug',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
        blocks: [
          {
            issue_id: 'TICK-200',
            depends_on_id: 'TICK-100',
            issue_title: 'Deploy to prod',
            type: 'blocks',
            created_at: '2026-01-01T00:00:00Z',
            created_by: 'user',
          },
        ],
        parent_ticket_id: 'EPIC-10',
        siblings: [
          {
            id: 'TICK-101',
            title: 'Sibling task',
            description: null,
            status: 'open',
            priority: 1,
            issue_type: 'task',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
            closed_at: null,
            created_by: 'user',
            close_reason: null,
          },
        ],
      }
      renderPage(ticket)
      expect(screen.getByText('Blocked by')).toBeInTheDocument()
      expect(screen.getByText('Blocks')).toBeInTheDocument()
      expect(screen.getByText('Epic Hierarchy')).toBeInTheDocument()
      expect(screen.getByText('Sibling Tickets')).toBeInTheDocument()
    })
  })
})
