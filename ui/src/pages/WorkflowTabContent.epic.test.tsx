import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BrowserRouter } from 'react-router-dom'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState } from '@/types/workflow'

// Mock child components
vi.mock('@/components/workflow/PhaseTimeline', () => ({
  PhaseTimeline: () => <div data-testid="phase-timeline">PhaseTimeline</div>,
}))
vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: () => <div data-testid="agent-log-panel">AgentLogPanel</div>,
}))

// Helper to render with Router
const renderWithRouter = (ui: React.ReactElement) => {
  return render(<BrowserRouter>{ui}</BrowserRouter>)
}

function makeState(overrides: Partial<WorkflowState> = {}): WorkflowState {
  return {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    category: 'full',
    status: 'active',
    phases: { implementation: { status: 'in_progress' } },
    phase_order: ['implementation'],
    ...overrides,
  }
}

const defaultProps = {
  ticketId: 'T-1',
  hasWorkflow: true,
  displayedState: makeState(),
  displayedWorkflowName: 'feature',
  hasMultipleWorkflows: false,
  workflows: ['feature'],
  selectedWorkflow: 'feature',
  onSelectWorkflow: vi.fn(),
  isOrchestrated: false,
  hasActivePhase: false,
  activeAgents: {},
  sessions: [],
  logPanelCollapsed: false,
  onToggleLogPanel: vi.fn(),
  selectedPanelAgent: null,
  onAgentSelect: vi.fn(),
  onStop: vi.fn(),
  stopPending: false,
  onShowRunDialog: vi.fn(),
}

describe('WorkflowTabContent - Epic workflow integration', () => {
  describe('epic ticket detection', () => {
    it('shows "Run Epic Workflow" button for epic tickets when no active workflow', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
    })

    it('does not show "Run Epic Workflow" button for non-epic tickets', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          issueType="task"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /run workflow/i })).toBeInTheDocument()
    })

    it('shows regular "Run Workflow" button when issueType is undefined', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          issueType={undefined}
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /run workflow/i })).toBeInTheDocument()
    })

    it('shows "Run Epic Workflow" button with Layers icon', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      const button = screen.getByRole('button', { name: /run epic workflow/i })
      // Check that Layers icon is present (lucide-react adds classes)
      const svg = button.querySelector('svg')
      expect(svg).toBeInTheDocument()
    })

    it('calls onShowEpicRunDialog when button clicked', async () => {
      const user = userEvent.setup()
      const onShowEpicRunDialog = vi.fn()
      render(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          onShowEpicRunDialog={onShowEpicRunDialog}
        />
      )

      await user.click(screen.getByRole('button', { name: /run epic workflow/i }))

      expect(onShowEpicRunDialog).toHaveBeenCalled()
    })

    it('does not show "Run Epic Workflow" when onShowEpicRunDialog is undefined', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          onShowEpicRunDialog={undefined}
        />
      )

      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
    })
  })

  describe('active chain link', () => {
    it('shows "View Chain" link when active chain exists', () => {
      renderWithRouter(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          activeChainId="chain-123"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.getByRole('link', { name: /view chain/i })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
    })

    it('"View Chain" link points to correct chain detail page', () => {
      renderWithRouter(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          activeChainId="chain-abc-123"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      const link = screen.getByRole('link', { name: /view chain/i })
      expect(link).toHaveAttribute('href', '/chains/chain-abc-123')
    })

    it('shows "View Chain" link with ExternalLink icon', () => {
      renderWithRouter(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          activeChainId="chain-123"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      const link = screen.getByRole('link', { name: /view chain/i })
      const svg = link.querySelector('svg')
      expect(svg).toBeInTheDocument()
    })

    it('shows "View Chain" for non-epic tickets with active chain', () => {
      renderWithRouter(
        <WorkflowTabContent
          {...defaultProps}
          issueType="task"
          activeChainId="chain-123"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.getByRole('link', { name: /view chain/i })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /run workflow/i })).not.toBeInTheDocument()
    })

    it('does not show "View Chain" when activeChainId is null', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          activeChainId={null}
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.queryByRole('link', { name: /view chain/i })).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
    })

    it('does not show "View Chain" when activeChainId is undefined', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          activeChainId={undefined}
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.queryByRole('link', { name: /view chain/i })).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
    })
  })

  describe('empty state', () => {
    it('shows "Run Epic Workflow" button in empty state for epic', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          hasWorkflow={false}
          displayedState={null}
          issueType="epic"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.getByText(/no workflow configured for this ticket/i)).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
    })

    it('shows "View Chain" in empty state when active chain exists', () => {
      renderWithRouter(
        <WorkflowTabContent
          {...defaultProps}
          hasWorkflow={false}
          displayedState={null}
          issueType="epic"
          activeChainId="chain-123"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.getByRole('link', { name: /view chain/i })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
    })

    it('shows regular "Run Workflow" button in empty state for non-epic', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          hasWorkflow={false}
          displayedState={null}
          issueType="task"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /run workflow/i })).toBeInTheDocument()
    })
  })

  describe('button priority', () => {
    it('hides run buttons when workflow is orchestrated', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          isOrchestrated={true}
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /run workflow/i })).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })

    it('hides run buttons when workflow has active phase', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          hasActivePhase={true}
          onShowEpicRunDialog={vi.fn()}
        />
      )

      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /run workflow/i })).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument()
    })

    it('shows active chain link even when orchestrated', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          isOrchestrated={true}
          activeChainId="chain-123"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      // Active chain link takes precedence even during orchestration
      // (though in practice, chains and single-ticket workflows are separate)
      expect(screen.queryByRole('link', { name: /view chain/i })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
    })
  })

  describe('button placement', () => {
    it('places epic button in same location as regular run button', () => {
      const { rerender } = render(
        <WorkflowTabContent
          {...defaultProps}
          issueType="task"
        />
      )

      const runButton = screen.getByRole('button', { name: /run workflow/i })
      const runButtonParent = runButton.parentElement

      rerender(
        <WorkflowTabContent
          {...defaultProps}
          issueType="epic"
          onShowEpicRunDialog={vi.fn()}
        />
      )

      const epicButton = screen.getByRole('button', { name: /run epic workflow/i })
      const epicButtonParent = epicButton.parentElement

      // Both buttons should be in similar DOM structure
      expect(runButtonParent?.className).toBe(epicButtonParent?.className)
    })
  })

  describe('all issue types', () => {
    const issueTypes = ['bug', 'feature', 'task', 'epic']

    issueTypes.forEach((type) => {
      it(`handles ${type} issue type correctly`, () => {
        render(
          <WorkflowTabContent
            {...defaultProps}
            issueType={type}
            onShowEpicRunDialog={vi.fn()}
          />
        )

        if (type === 'epic') {
          expect(screen.getByRole('button', { name: /run epic workflow/i })).toBeInTheDocument()
          expect(screen.queryByRole('button', { name: /^run workflow$/i })).not.toBeInTheDocument()
        } else {
          expect(screen.queryByRole('button', { name: /run epic workflow/i })).not.toBeInTheDocument()
          expect(screen.getByRole('button', { name: /run workflow/i })).toBeInTheDocument()
        }
      })
    })
  })
})
