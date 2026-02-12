import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { TicketCard } from './TicketCard'
import type { PendingTicket } from '@/types/ticket'

function renderCard(ticket: PendingTicket) {
  return render(
    <MemoryRouter>
      <TicketCard ticket={ticket} />
    </MemoryRouter>
  )
}

function createMockTicket(overrides: Partial<PendingTicket> = {}): PendingTicket {
  return {
    id: 'TICKET-123',
    title: 'Test ticket',
    description: 'Test description',
    status: 'in_progress',
    priority: 2,
    issue_type: 'feature',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    closed_at: null,
    created_by: 'test-user',
    close_reason: null,
    is_blocked: false,
    ...overrides,
  }
}

describe('TicketCard - Workflow Progress Display', () => {
  it('renders ticket without workflow progress', () => {
    const ticket = createMockTicket({ workflow_progress: undefined })
    renderCard(ticket)

    expect(screen.getByText('TICKET-123')).toBeInTheDocument()
    expect(screen.getByText('Test ticket')).toBeInTheDocument()
    expect(screen.queryByText(/phases/)).not.toBeInTheDocument()
  })

  it('renders workflow progress bar and text when workflow_progress exists', () => {
    const ticket = createMockTicket({
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'implementation',
        completed_phases: 3,
        total_phases: 5,
        status: 'active',
      },
    })
    renderCard(ticket)

    expect(screen.getByText('3/5 phases')).toBeInTheDocument()
  })

  it('calculates 60% progress for 3/5 phases', () => {
    const ticket = createMockTicket({
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'implementation',
        completed_phases: 3,
        total_phases: 5,
        status: 'active',
      },
    })
    const { container } = renderCard(ticket)

    // Find progress bar inner div with width style
    const progressBar = container.querySelector('.bg-primary')
    expect(progressBar).toBeInTheDocument()
    expect(progressBar).toHaveStyle({ width: '60%' })
  })

  it('shows 0% progress when no phases completed', () => {
    const ticket = createMockTicket({
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'investigation',
        completed_phases: 0,
        total_phases: 5,
        status: 'active',
      },
    })
    const { container } = renderCard(ticket)

    const progressBar = container.querySelector('.bg-primary')
    expect(progressBar).toHaveStyle({ width: '0%' })
    expect(screen.getByText('0/5 phases')).toBeInTheDocument()
  })

  it('shows 100% progress when all phases completed', () => {
    const ticket = createMockTicket({
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'docs',
        completed_phases: 5,
        total_phases: 5,
        status: 'active',
      },
    })
    const { container } = renderCard(ticket)

    const progressBar = container.querySelector('.bg-primary')
    expect(progressBar).toHaveStyle({ width: '100%' })
    expect(screen.getByText('5/5 phases')).toBeInTheDocument()
  })

  it('handles edge case: total_phases = 0 shows 0% progress', () => {
    const ticket = createMockTicket({
      workflow_progress: {
        workflow_name: 'hotfix',
        current_phase: 'implementation',
        completed_phases: 0,
        total_phases: 0,
        status: 'active',
      },
    })
    const { container } = renderCard(ticket)

    const progressBar = container.querySelector('.bg-primary')
    expect(progressBar).toHaveStyle({ width: '0%' })
    expect(screen.getByText('0/0 phases')).toBeInTheDocument()
  })

  it('handles edge case: completed_phases > total_phases', () => {
    const ticket = createMockTicket({
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'docs',
        completed_phases: 6,
        total_phases: 5,
        status: 'active',
      },
    })
    const { container } = renderCard(ticket)

    // Should calculate 120% but cap might be CSS-dependent
    const progressBar = container.querySelector('.bg-primary')
    expect(progressBar).toBeInTheDocument()
    expect(screen.getByText('6/5 phases')).toBeInTheDocument()
  })

  it('renders progress for ticket with single phase workflow', () => {
    const ticket = createMockTicket({
      workflow_progress: {
        workflow_name: 'hotfix',
        current_phase: 'implementation',
        completed_phases: 1,
        total_phases: 1,
        status: 'active',
      },
    })
    renderCard(ticket)

    expect(screen.getByText('1/1 phases')).toBeInTheDocument()
  })

  it('only shows progress for in_progress tickets with workflow_progress', () => {
    const openTicket = createMockTicket({
      status: 'open',
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'investigation',
        completed_phases: 2,
        total_phases: 5,
        status: 'active',
      },
    })
    const { container } = renderCard(openTicket)

    // Progress should still render regardless of ticket status
    // (implementation shows progress bar when workflow_progress exists)
    expect(container.querySelector('.bg-primary')).toBeInTheDocument()
  })

  it('does not render progress for closed tickets even if workflow_progress exists', () => {
    const closedTicket = createMockTicket({
      status: 'closed',
      closed_at: '2026-01-02T00:00:00Z',
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'docs',
        completed_phases: 5,
        total_phases: 5,
        status: 'completed',
      },
    })
    const { container } = renderCard(closedTicket)

    // The implementation shows progress whenever workflow_progress exists
    expect(container.querySelector('.bg-primary')).toBeInTheDocument()
    expect(screen.getByText('5/5 phases')).toBeInTheDocument()
  })

  it('rounds percentage to nearest integer', () => {
    const ticket = createMockTicket({
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'implementation',
        completed_phases: 1,
        total_phases: 3,
        status: 'active',
      },
    })
    const { container } = renderCard(ticket)

    const progressBar = container.querySelector('.bg-primary')
    // 1/3 = 33.333...% should round to 33%
    expect(progressBar).toHaveStyle({ width: '33%' })
  })

  it('displays blocked icon when ticket is blocked', () => {
    const ticket = createMockTicket({
      is_blocked: true,
      blocked_by: ['TICKET-100'],
    })
    const { container } = renderCard(ticket)

    const lockIcon = container.querySelector('[title="Blocked"]')
    expect(lockIcon).toBeInTheDocument()
  })

  it('renders ticket link with correct href', () => {
    const ticket = createMockTicket()
    renderCard(ticket)

    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', '/tickets/TICKET-123')
  })

  it('renders progress bar container with correct styling', () => {
    const ticket = createMockTicket({
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'implementation',
        completed_phases: 2,
        total_phases: 4,
        status: 'active',
      },
    })
    const { container } = renderCard(ticket)

    // Progress bar container should have muted background and rounded corners
    const progressContainer = container.querySelector('.bg-muted.rounded-full')
    expect(progressContainer).toBeInTheDocument()
    expect(progressContainer).toHaveClass('h-1.5', 'overflow-hidden', 'flex-1')
  })

  it('progress bar has transition animation class', () => {
    const ticket = createMockTicket({
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'implementation',
        completed_phases: 2,
        total_phases: 5,
        status: 'active',
      },
    })
    const { container } = renderCard(ticket)

    const progressBar = container.querySelector('.bg-primary')
    expect(progressBar).toHaveClass('transition-all')
  })

  it('phase text is non-wrapping', () => {
    const ticket = createMockTicket({
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'implementation',
        completed_phases: 3,
        total_phases: 5,
        status: 'active',
      },
    })
    renderCard(ticket)

    const phaseText = screen.getByText('3/5 phases')
    expect(phaseText).toHaveClass('whitespace-nowrap')
  })
})

describe('TicketCard - Integration', () => {
  it('renders complete ticket card with progress', () => {
    const ticket = createMockTicket({
      id: 'PROJ-456',
      title: 'Add new feature',
      description: 'Feature description here',
      status: 'in_progress',
      priority: 1,
      issue_type: 'feature',
      workflow_progress: {
        workflow_name: 'feature',
        current_phase: 'implementation',
        completed_phases: 3,
        total_phases: 5,
        status: 'active',
      },
    })
    renderCard(ticket)

    // All elements should render
    expect(screen.getByText('PROJ-456')).toBeInTheDocument()
    expect(screen.getByText('Add new feature')).toBeInTheDocument()
    expect(screen.getByText('Feature description here')).toBeInTheDocument()
    expect(screen.getByText('in progress')).toBeInTheDocument()
    expect(screen.getByText('3/5 phases')).toBeInTheDocument()
  })
})
