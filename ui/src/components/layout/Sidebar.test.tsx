import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Sidebar } from './Sidebar'
import type { StatusResponse } from '@/types/ticket'

// Mock useStatus and useProjectWorkflow hooks
const mockUseStatus = vi.fn()
const mockUseProjectWorkflow = vi.fn()
vi.mock('@/hooks/useTickets', () => ({
  useStatus: () => mockUseStatus(),
  useProjectWorkflow: () => mockUseProjectWorkflow(),
}))

const mockUseChainList = vi.fn()
vi.mock('@/hooks/useChains', () => ({
  useChainList: () => mockUseChainList(),
}))

function renderSidebar(initialRoute = '/') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialRoute]}>
        <Sidebar />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

function createMockStatus(overrides: Partial<StatusResponse> = {}): StatusResponse {
  return {
    counts: {
      open: 5,
      in_progress: 0,
      closed: 10,
      blocked: 2,
      total: 17,
    },
    ready_count: 5,
    pending_tickets: [],
    recent_closed: [],
    ...overrides,
  }
}

describe('Sidebar - Spinner Visibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseProjectWorkflow.mockReturnValue({ data: undefined })
    mockUseChainList.mockReturnValue({ data: [] })
  })

  it('shows spinner when in_progress count > 0', () => {
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: {
          open: 5,
          in_progress: 3,
          closed: 10,
          blocked: 2,
          total: 20,
        },
      }),
    })

    const { container } = renderSidebar()

    // Spinner should be visible
    const spinner = container.querySelector('[class*="spin-sync"]')
    expect(spinner).toBeInTheDocument()
  })

  it('hides spinner when in_progress count = 0', () => {
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: {
          open: 5,
          in_progress: 0,
          closed: 10,
          blocked: 2,
          total: 17,
        },
      }),
    })

    const { container } = renderSidebar()

    // Spinner should not be visible
    const spinner = container.querySelector('[class*="spin-sync"]')
    expect(spinner).not.toBeInTheDocument()
  })

  it('hides spinner when status data is undefined', () => {
    mockUseStatus.mockReturnValue({ data: undefined })

    const { container } = renderSidebar()

    // Spinner should not be visible
    const spinner = container.querySelector('[class*="spin-sync"]')
    expect(spinner).not.toBeInTheDocument()
  })

  it('shows spinner with in_progress count = 1', () => {
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: {
          open: 5,
          in_progress: 1,
          closed: 10,
          blocked: 2,
          total: 18,
        },
      }),
    })

    const { container } = renderSidebar()

    const spinner = container.querySelector('[class*="spin-sync"]')
    expect(spinner).toBeInTheDocument()
  })

  it('shows spinner with large in_progress count', () => {
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: {
          open: 5,
          in_progress: 100,
          closed: 10,
          blocked: 2,
          total: 117,
        },
      }),
    })

    const { container } = renderSidebar()

    const spinner = container.querySelector('[class*="spin-sync"]')
    expect(spinner).toBeInTheDocument()
  })

  it('spinner appears between label and count in In Progress nav item', () => {
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: {
          open: 5,
          in_progress: 3,
          closed: 10,
          blocked: 2,
          total: 20,
        },
      }),
    })

    renderSidebar()

    // Find the In Progress nav item
    const inProgressLink = screen.getByRole('link', { name: /in progress/i })
    expect(inProgressLink).toBeInTheDocument()

    // Check structure: label, spinner, count
    const textContent = inProgressLink.textContent
    expect(textContent).toContain('In Progress')
    expect(textContent).toContain('3')
  })

  it('count is still displayed when spinner is shown', () => {
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: {
          open: 5,
          in_progress: 7,
          closed: 10,
          blocked: 2,
          total: 24,
        },
      }),
    })

    renderSidebar()

    const inProgressLink = screen.getByRole('link', { name: /in progress/i })
    expect(inProgressLink.textContent).toContain('7')
  })
})

describe('Sidebar - Navigation', () => {
  beforeEach(() => {
    mockUseProjectWorkflow.mockReturnValue({ data: undefined })
    mockUseChainList.mockReturnValue({ data: [] })
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: {
          open: 5,
          in_progress: 3,
          closed: 10,
          blocked: 2,
          total: 20,
        },
      }),
    })
  })

  it('renders all navigation items', () => {
    renderSidebar()

    expect(screen.getByRole('link', { name: /dashboard/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /all tickets/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /new ticket/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /^open/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /in progress/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /closed/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /blocked/i })).toBeInTheDocument()
  })

  it('highlights active route on dashboard', () => {
    renderSidebar('/')

    const dashboardLink = screen.getByRole('link', { name: /dashboard/i })
    expect(dashboardLink).toHaveClass('bg-muted', 'text-foreground')
  })

  it('highlights active route on tickets list', () => {
    renderSidebar('/tickets')

    const ticketsLink = screen.getByRole('link', { name: /all tickets/i })
    expect(ticketsLink).toHaveClass('bg-muted', 'text-foreground')
  })

  it('highlights active route on in progress filter', () => {
    renderSidebar('/tickets?status=in_progress')

    const inProgressLink = screen.getByRole('link', { name: /in progress/i })
    expect(inProgressLink).toHaveClass('bg-muted', 'text-foreground')
  })

  it('displays correct counts for all status items', () => {
    renderSidebar()

    const allTickets = screen.getByRole('link', { name: /all tickets/i })
    expect(allTickets.textContent).toContain('20')

    const openLink = screen.getByRole('link', { name: /^open/i })
    expect(openLink.textContent).toContain('5')

    const inProgressLink = screen.getByRole('link', { name: /in progress/i })
    expect(inProgressLink.textContent).toContain('3')

    const closedLink = screen.getByRole('link', { name: /closed/i })
    expect(closedLink.textContent).toContain('10')

    const blockedLink = screen.getByRole('link', { name: /blocked/i })
    expect(blockedLink.textContent).toContain('2')
  })
})

describe('Sidebar - Spinner Component Properties', () => {
  beforeEach(() => {
    mockUseProjectWorkflow.mockReturnValue({ data: undefined })
    mockUseChainList.mockReturnValue({ data: [] })
  })

  it('uses Spinner component with size="sm"', () => {
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: {
          open: 5,
          in_progress: 1,
          closed: 10,
          blocked: 2,
          total: 18,
        },
      }),
    })

    const { container } = renderSidebar()

    // Find spinner element - Spinner with size="sm" should have specific classes
    const spinner = container.querySelector('[class*="spin-sync"]')
    expect(spinner).toBeInTheDocument()

    // Check for small size class (h-4 w-4 from Spinner size="sm")
    expect(spinner).toHaveClass('h-4', 'w-4')
  })

  it('spinner does not appear in other nav items', () => {
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: {
          open: 5,
          in_progress: 1,
          closed: 10,
          blocked: 2,
          total: 18,
        },
      }),
    })

    const { container } = renderSidebar()

    // Only one spinner should exist (in In Progress item)
    const spinners = container.querySelectorAll('[class*="spin-sync"]')
    expect(spinners).toHaveLength(1)
  })
})

describe('Sidebar - Edge Cases', () => {
  beforeEach(() => {
    mockUseProjectWorkflow.mockReturnValue({ data: undefined })
    mockUseChainList.mockReturnValue({ data: [] })
  })

  it('handles missing counts gracefully', () => {
    mockUseStatus.mockReturnValue({
      data: {
        counts: {
          open: 0,
          in_progress: 0,
          closed: 0,
          blocked: 0,
          total: 0,
        },
        ready_count: 0,
        pending_tickets: [],
        recent_closed: [],
      },
    })

    const { container } = renderSidebar()

    // No spinner when in_progress = 0
    const spinner = container.querySelector('[class*="spin-sync"]')
    expect(spinner).not.toBeInTheDocument()

    // All nav items should still render with 0 counts
    expect(screen.getByRole('link', { name: /all tickets/i }).textContent).toContain('0')
    expect(screen.getByRole('link', { name: /in progress/i }).textContent).toContain('0')
  })

  it('renders without status data', () => {
    mockUseStatus.mockReturnValue({ data: undefined })

    renderSidebar()

    // Should still render navigation items, just without counts
    expect(screen.getByRole('link', { name: /dashboard/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /in progress/i })).toBeInTheDocument()
  })

  it('spinner visibility updates when status changes', () => {
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: {
          open: 5,
          in_progress: 0,
          closed: 10,
          blocked: 2,
          total: 17,
        },
      }),
    })

    const { container, rerender } = renderSidebar()

    // No spinner initially
    let spinner = container.querySelector('[class*="spin-sync"]')
    expect(spinner).not.toBeInTheDocument()

    // Update to have in_progress > 0
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: {
          open: 4,
          in_progress: 1,
          closed: 10,
          blocked: 2,
          total: 17,
        },
      }),
    })

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/']}>
          <Sidebar />
        </MemoryRouter>
      </QueryClientProvider>
    )

    // Spinner should now appear
    spinner = container.querySelector('[class*="spin-sync"]')
    expect(spinner).toBeInTheDocument()
  })
})

describe('Sidebar - Project Workflow Spinner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseStatus.mockReturnValue({ data: createMockStatus() })
    mockUseChainList.mockReturnValue({ data: [] })
  })

  it('shows spinner on Project Workflows nav item when a workflow is active', () => {
    mockUseProjectWorkflow.mockReturnValue({
      data: { all_workflows: { 'inst-1': { status: 'active' } } },
    })

    const { container } = renderSidebar()

    const projectWorkflowsLink = screen.getByRole('link', { name: /project workflows/i })
    const spinner = projectWorkflowsLink.querySelector('[class*="spin-sync"]')
    expect(spinner).toBeInTheDocument()
  })

  it('hides spinner on Project Workflows nav item when all workflows are completed', () => {
    mockUseProjectWorkflow.mockReturnValue({
      data: {
        all_workflows: {
          'inst-1': { status: 'completed' },
          'inst-2': { status: 'failed' },
          'inst-3': { status: 'project_completed' },
        },
      },
    })

    const { container } = renderSidebar()

    const projectWorkflowsLink = screen.getByRole('link', { name: /project workflows/i })
    const spinner = projectWorkflowsLink.querySelector('[class*="spin-sync"]')
    expect(spinner).not.toBeInTheDocument()
  })

  it('hides spinner when all_workflows is undefined', () => {
    mockUseProjectWorkflow.mockReturnValue({ data: undefined })

    const { container } = renderSidebar()

    const projectWorkflowsLink = screen.getByRole('link', { name: /project workflows/i })
    const spinner = projectWorkflowsLink.querySelector('[class*="spin-sync"]')
    expect(spinner).not.toBeInTheDocument()
  })

  it('hides spinner when all_workflows is empty', () => {
    mockUseProjectWorkflow.mockReturnValue({ data: { all_workflows: {} } })

    const { container } = renderSidebar()

    const projectWorkflowsLink = screen.getByRole('link', { name: /project workflows/i })
    const spinner = projectWorkflowsLink.querySelector('[class*="spin-sync"]')
    expect(spinner).not.toBeInTheDocument()
  })

  it('shows spinner when at least one workflow is active among mixed statuses', () => {
    mockUseProjectWorkflow.mockReturnValue({
      data: {
        all_workflows: {
          'inst-1': { status: 'completed' },
          'inst-2': { status: 'active' },
          'inst-3': { status: 'failed' },
        },
      },
    })

    renderSidebar()

    const link = screen.getByRole('link', { name: /project workflows/i })
    const spinner = link.querySelector('[class*="spin-sync"]')
    expect(spinner).toBeInTheDocument()
  })

  it('two spinners appear when both in_progress tickets and active project workflow exist', () => {
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: { open: 0, in_progress: 1, closed: 0, blocked: 0, total: 1 },
      }),
    })
    mockUseProjectWorkflow.mockReturnValue({
      data: { all_workflows: { 'inst-1': { status: 'active' } } },
    })

    const { container } = renderSidebar()

    const spinners = container.querySelectorAll('[class*="spin-sync"]')
    expect(spinners).toHaveLength(2)
  })
})

describe('Sidebar - Chain Execution Spinner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseStatus.mockReturnValue({ data: createMockStatus() })
    mockUseProjectWorkflow.mockReturnValue({ data: undefined })
  })

  it('shows spinner and remaining count on Chain Executions nav item when chains are running', () => {
    mockUseChainList.mockReturnValue({ data: [{ id: 'c1', status: 'running', total_items: 3, completed_items: 1 }] })

    renderSidebar()

    const link = screen.getByRole('link', { name: /chain executions/i })
    const spinner = link.querySelector('[class*="spin-sync"]')
    expect(spinner).toBeInTheDocument()
    // remaining = 3 - 1 = 2
    expect(link.textContent).toContain('2')
  })

  it('shows summed remaining count across multiple running chains', () => {
    mockUseChainList.mockReturnValue({
      data: [
        { id: 'c1', status: 'running', total_items: 5, completed_items: 2 },
        { id: 'c2', status: 'running', total_items: 3, completed_items: 1 },
      ],
    })

    renderSidebar()

    const link = screen.getByRole('link', { name: /chain executions/i })
    // remaining = (5-2) + (3-1) = 3 + 2 = 5
    expect(link.textContent).toContain('5')
    expect(link.querySelector('[class*="spin-sync"]')).toBeInTheDocument()
  })

  it('shows count of 0 when all chain items are completed but chain is still running', () => {
    mockUseChainList.mockReturnValue({ data: [{ id: 'c1', status: 'running', total_items: 3, completed_items: 3 }] })

    renderSidebar()

    const link = screen.getByRole('link', { name: /chain executions/i })
    expect(link.textContent).toContain('0')
    expect(link.querySelector('[class*="spin-sync"]')).toBeInTheDocument()
  })

  it('hides spinner and count when no chains are running', () => {
    mockUseChainList.mockReturnValue({ data: [] })

    renderSidebar()

    const link = screen.getByRole('link', { name: /chain executions/i })
    expect(link.querySelector('[class*="spin-sync"]')).not.toBeInTheDocument()
    expect(link.textContent?.replace(/\D/g, '')).toBe('')
  })

  it('hides spinner and count when chain data is undefined', () => {
    mockUseChainList.mockReturnValue({ data: undefined })

    renderSidebar()

    const link = screen.getByRole('link', { name: /chain executions/i })
    expect(link.querySelector('[class*="spin-sync"]')).not.toBeInTheDocument()
    expect(link.textContent?.replace(/\D/g, '')).toBe('')
  })

  it('count appears before spinner in DOM order', () => {
    mockUseChainList.mockReturnValue({ data: [{ id: 'c1', status: 'running', total_items: 4, completed_items: 1 }] })

    renderSidebar()

    const link = screen.getByRole('link', { name: /chain executions/i })
    const spinner = link.querySelector('[class*="spin-sync"]')!
    const countSpan = link.querySelector('span.text-xs.text-muted-foreground')!
    expect(countSpan).toBeInTheDocument()
    // count span must precede spinner in DOM
    expect(countSpan.compareDocumentPosition(spinner) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('shows three spinners when in_progress tickets, active project workflow, and running chains all exist', () => {
    mockUseStatus.mockReturnValue({
      data: createMockStatus({
        counts: { open: 0, in_progress: 1, closed: 0, blocked: 0, total: 1 },
      }),
    })
    mockUseProjectWorkflow.mockReturnValue({
      data: { all_workflows: { 'inst-1': { status: 'active' } } },
    })
    mockUseChainList.mockReturnValue({ data: [{ id: 'c1', status: 'running', total_items: 5, completed_items: 2 }] })

    const { container } = renderSidebar()

    const spinners = container.querySelectorAll('[class*="spin-sync"]')
    expect(spinners).toHaveLength(3)
  })
})
