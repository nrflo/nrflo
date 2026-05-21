import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState, PauseResult } from '@/types/workflow'

vi.mock('@/components/workflow/PhaseTimeline', () => ({
  PhaseTimeline: () => <div data-testid="phase-timeline">PhaseTimeline</div>,
}))
vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: () => <div data-testid="agent-log-panel">AgentLogPanel</div>,
}))

function makeState(overrides: Partial<WorkflowState> = {}): WorkflowState {
  return {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    status: 'active',
    phases: { implementation: { status: 'in_progress' } },
    phase_order: ['implementation'],
    ...overrides,
  }
}

function makePauseResult(overrides: Partial<PauseResult> = {}): PauseResult {
  return {
    paused_after_layer: 0,
    resume_layer: 1,
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

describe('WorkflowTabContent - Pause controls', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('waiting status — WorkflowPauseControls', () => {
    it('shows Waiting badge when status is waiting and onContinue is provided', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          onContinue={vi.fn()}
        />
      )
      expect(screen.getByText('Waiting')).toBeInTheDocument()
    })

    it('shows pause message when status is waiting', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          onContinue={vi.fn()}
        />
      )
      expect(screen.getByText(/workflow paused/i)).toBeInTheDocument()
    })

    it('does not show Waiting badge when status is active', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'active' })}
          onContinue={vi.fn()}
        />
      )
      expect(screen.queryByText('Waiting')).not.toBeInTheDocument()
    })

    it('does not show pause controls when onContinue is not provided', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
        />
      )
      expect(screen.queryByText('Waiting')).not.toBeInTheDocument()
    })

    it('shows instructions textarea when status is waiting', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          onContinue={vi.fn()}
        />
      )
      expect(screen.getByPlaceholderText(/optional instructions/i)).toBeInTheDocument()
    })

    it('shows Continue button when status is waiting', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          onContinue={vi.fn()}
        />
      )
      expect(screen.getByRole('button', { name: /continue/i })).toBeInTheDocument()
    })

    it('calls onContinue with empty instructions when Continue clicked without input', async () => {
      const user = userEvent.setup()
      const onContinue = vi.fn()
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          onContinue={onContinue}
        />
      )

      await user.click(screen.getByRole('button', { name: /continue/i }))
      expect(onContinue).toHaveBeenCalledWith('')
    })

    it('calls onContinue with typed instructions', async () => {
      const user = userEvent.setup()
      const onContinue = vi.fn()
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          onContinue={onContinue}
        />
      )

      await user.type(screen.getByPlaceholderText(/optional instructions/i), 'do it differently')
      await user.click(screen.getByRole('button', { name: /continue/i }))
      expect(onContinue).toHaveBeenCalledWith('do it differently')
    })

    it('disables Continue button while continuePending', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          onContinue={vi.fn()}
          continuePending={true}
        />
      )
      expect(screen.getByRole('button', { name: /continue/i })).toBeDisabled()
    })

    it('shows Fail Workflow button in waiting state when onFail provided', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          onContinue={vi.fn()}
          onFail={vi.fn()}
        />
      )
      expect(screen.getByRole('button', { name: /fail workflow/i })).toBeInTheDocument()
    })

    it('shows fail reason input in waiting state when onFail provided', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          onContinue={vi.fn()}
          onFail={vi.fn()}
        />
      )
      expect(screen.getByPlaceholderText(/reason to fail/i)).toBeInTheDocument()
    })

    it('Fail Workflow button is disabled when no reason entered', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          onContinue={vi.fn()}
          onFail={vi.fn()}
        />
      )
      expect(screen.getByRole('button', { name: /fail workflow/i })).toBeDisabled()
    })

    it('calls onFail with reason after confirm when waiting', async () => {
      const user = userEvent.setup()
      const onFail = vi.fn()
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          onContinue={vi.fn()}
          onFail={onFail}
        />
      )

      await user.type(screen.getByPlaceholderText(/reason to fail/i), 'cancelled')
      // Fail button is now enabled
      const failBtn = screen.getByRole('button', { name: /fail workflow/i })
      expect(failBtn).not.toBeDisabled()
      await user.click(failBtn)
      // Confirm dialog appears
      await user.click(screen.getByRole('button', { name: /^fail$/i }))
      expect(onFail).toHaveBeenCalledWith('cancelled')
    })
  })

  describe('running status — WorkflowFailControl', () => {
    it('shows Fail button in running state when isOrchestrated and onFail provided', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'active' })}
          isOrchestrated={true}
          onFail={vi.fn()}
        />
      )
      expect(screen.getByRole('button', { name: /^fail$/i })).toBeInTheDocument()
    })

    it('shows Fail button when hasActivePhase and onFail provided', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'active' })}
          hasActivePhase={true}
          onFail={vi.fn()}
        />
      )
      expect(screen.getByRole('button', { name: /^fail$/i })).toBeInTheDocument()
    })

    it('does not show WorkflowFailControl when status is waiting', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'waiting' })}
          isOrchestrated={true}
          onContinue={vi.fn()}
          onFail={vi.fn()}
        />
      )
      // In waiting state, Fail is inside WorkflowPauseControls as "Fail Workflow", not standalone "Fail"
      expect(screen.queryByRole('button', { name: /^fail$/i })).not.toBeInTheDocument()
    })

    it('Fail button is disabled when no reason entered', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'active' })}
          isOrchestrated={true}
          onFail={vi.fn()}
        />
      )
      const failBtn = screen.getByRole('button', { name: /^fail$/i })
      expect(failBtn).toBeDisabled()
    })

    it('calls onFail after entering reason and confirming', async () => {
      const user = userEvent.setup()
      const onFail = vi.fn()
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'active' })}
          isOrchestrated={true}
          onFail={onFail}
        />
      )

      await user.type(screen.getByPlaceholderText(/fail reason/i), 'out of time')
      const failBtns = screen.getAllByRole('button', { name: /^fail$/i })
      expect(failBtns[0]).not.toBeDisabled()
      await user.click(failBtns[0])
      // Confirm dialog opens — click the dialog's Fail button
      const dialogFailBtn = screen.getAllByRole('button', { name: /^fail$/i }).at(-1)!
      await user.click(dialogFailBtn)
      expect(onFail).toHaveBeenCalledWith('out of time')
    })
  })

  describe('PauseResultPanel integration', () => {
    it('renders PauseResultPanel when pause_result is present', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({
            status: 'waiting',
            pause_result: makePauseResult({ paused_after_layer: 1, resume_layer: 2 }),
          })}
          onContinue={vi.fn()}
        />
      )
      expect(screen.getByText(/paused after layer 1/i)).toBeInTheDocument()
    })

    it('does not render PauseResultPanel when pause_result is absent', () => {
      render(
        <WorkflowTabContent
          {...defaultProps}
          displayedState={makeState({ status: 'active' })}
        />
      )
      expect(screen.queryByText(/paused after layer/i)).not.toBeInTheDocument()
    })
  })
})
