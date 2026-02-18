import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BrowserRouter } from 'react-router-dom'
import { WorkflowTabContent } from './WorkflowTabContent'
import type { WorkflowState, ActiveAgentV4 } from '@/types/workflow'

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

function makeClaudeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    pid: 12345,
    session_id: 'sess-abc-123',
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeGptAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a2',
    agent_type: 'tester',
    phase: 'verification',
    model_id: 'gpt-4',
    cli: 'openai',
    pid: 99999,
    session_id: 'sess-gpt-456',
    started_at: '2026-01-01T00:00:00Z',
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
  hasActivePhase: true,
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

function renderContent(overrides: Record<string, unknown> = {}) {
  const props = { ...defaultProps, ...overrides }
  return render(
    <BrowserRouter>
      <WorkflowTabContent {...(props as Parameters<typeof WorkflowTabContent>[0])} />
    </BrowserRouter>
  )
}

describe('WorkflowTabContent - Take Control button', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('button visibility', () => {
    it('shows Take Control button when onTakeControl provided and Claude agent is running', () => {
      renderContent({
        onTakeControl: vi.fn(),
        activeAgents: { 'impl:claude:sonnet': makeClaudeAgent() },
        hasActivePhase: true,
      })

      expect(screen.getByRole('button', { name: /take control/i })).toBeInTheDocument()
    })

    it('does NOT show Take Control button when no onTakeControl prop', () => {
      renderContent({
        activeAgents: { 'impl:claude:sonnet': makeClaudeAgent() },
        hasActivePhase: true,
        onTakeControl: undefined,
      })

      expect(screen.queryByRole('button', { name: /take control/i })).not.toBeInTheDocument()
    })

    it('does NOT show Take Control button when agent has no session_id', () => {
      renderContent({
        onTakeControl: vi.fn(),
        activeAgents: { 'impl:claude:sonnet': makeClaudeAgent({ session_id: undefined }) },
        hasActivePhase: true,
      })

      expect(screen.queryByRole('button', { name: /take control/i })).not.toBeInTheDocument()
    })

    it('does NOT show Take Control button for non-Claude agents (no cli=claude)', () => {
      renderContent({
        onTakeControl: vi.fn(),
        activeAgents: { 'tester:gpt:4': makeGptAgent() },
        hasActivePhase: true,
      })

      expect(screen.queryByRole('button', { name: /take control/i })).not.toBeInTheDocument()
    })

    it('does NOT show Take Control button when agent has a result (completed)', () => {
      renderContent({
        onTakeControl: vi.fn(),
        activeAgents: { 'impl:claude:sonnet': makeClaudeAgent({ result: 'pass' }) },
        hasActivePhase: false,
        isOrchestrated: false,
      })

      expect(screen.queryByRole('button', { name: /take control/i })).not.toBeInTheDocument()
    })

    it('does NOT show Take Control button when neither orchestrated nor hasActivePhase', () => {
      renderContent({
        onTakeControl: vi.fn(),
        activeAgents: { 'impl:claude:sonnet': makeClaudeAgent() },
        hasActivePhase: false,
        isOrchestrated: false,
      })

      expect(screen.queryByRole('button', { name: /take control/i })).not.toBeInTheDocument()
    })

    it('shows Take Control button when orchestrated (even with no active phase)', () => {
      renderContent({
        onTakeControl: vi.fn(),
        activeAgents: { 'impl:claude:sonnet': makeClaudeAgent() },
        hasActivePhase: false,
        isOrchestrated: true,
      })

      expect(screen.getByRole('button', { name: /take control/i })).toBeInTheDocument()
    })
  })

  describe('disabled state', () => {
    it('is disabled when takeControlPending is true', () => {
      renderContent({
        onTakeControl: vi.fn(),
        takeControlPending: true,
        activeAgents: { 'impl:claude:sonnet': makeClaudeAgent() },
        hasActivePhase: true,
      })

      expect(screen.getByRole('button', { name: /take control/i })).toBeDisabled()
    })

    it('is enabled when takeControlPending is false', () => {
      renderContent({
        onTakeControl: vi.fn(),
        takeControlPending: false,
        activeAgents: { 'impl:claude:sonnet': makeClaudeAgent() },
        hasActivePhase: true,
      })

      expect(screen.getByRole('button', { name: /take control/i })).not.toBeDisabled()
    })
  })

  describe('click behavior', () => {
    it('calls onTakeControl with the running agent session_id when clicked', async () => {
      const user = userEvent.setup()
      const onTakeControl = vi.fn()

      renderContent({
        onTakeControl,
        activeAgents: { 'impl:claude:sonnet': makeClaudeAgent({ session_id: 'my-session' }) },
        hasActivePhase: true,
      })

      await user.click(screen.getByRole('button', { name: /take control/i }))
      expect(onTakeControl).toHaveBeenCalledWith('my-session')
      expect(onTakeControl).toHaveBeenCalledTimes(1)
    })

    it('prefers selectedPanelAgent session when it is a running Claude agent', async () => {
      const user = userEvent.setup()
      const onTakeControl = vi.fn()
      const panelAgent = makeClaudeAgent({ session_id: 'panel-session', agent_id: 'panel-1' })

      renderContent({
        onTakeControl,
        activeAgents: {
          'impl:claude:sonnet': makeClaudeAgent({ session_id: 'fallback-session' }),
          'panel:claude:sonnet': panelAgent,
        },
        selectedPanelAgent: {
          agent: panelAgent,
          phaseName: 'implementation',
        },
        hasActivePhase: true,
      })

      await user.click(screen.getByRole('button', { name: /take control/i }))
      expect(onTakeControl).toHaveBeenCalledWith('panel-session')
    })

    it('falls back to first running Claude agent when selectedPanelAgent is null', async () => {
      const user = userEvent.setup()
      const onTakeControl = vi.fn()

      renderContent({
        onTakeControl,
        activeAgents: { 'impl:claude:sonnet': makeClaudeAgent({ session_id: 'fallback-session' }) },
        selectedPanelAgent: null,
        hasActivePhase: true,
      })

      await user.click(screen.getByRole('button', { name: /take control/i }))
      expect(onTakeControl).toHaveBeenCalledWith('fallback-session')
    })

    it('falls back to running Claude agent when selectedPanelAgent is a completed agent', async () => {
      const user = userEvent.setup()
      const onTakeControl = vi.fn()
      const completedAgent = makeClaudeAgent({ session_id: 'completed-session', result: 'pass' })

      renderContent({
        onTakeControl,
        activeAgents: {
          'impl:claude:sonnet': makeClaudeAgent({ session_id: 'running-session' }),
        },
        selectedPanelAgent: {
          agent: completedAgent,
          phaseName: 'implementation',
        },
        hasActivePhase: true,
      })

      await user.click(screen.getByRole('button', { name: /take control/i }))
      expect(onTakeControl).toHaveBeenCalledWith('running-session')
    })
  })

  describe('placement relative to Stop button', () => {
    it('renders Take Control button in the same flex container as Stop button', () => {
      renderContent({
        onTakeControl: vi.fn(),
        activeAgents: { 'impl:claude:sonnet': makeClaudeAgent() },
        hasActivePhase: true,
      })

      const stopButton = screen.getByRole('button', { name: /stop/i })
      const takeControlButton = screen.getByRole('button', { name: /take control/i })

      // Both should be within the same flex items-center gap-3 container
      // (the left-side header container)
      const stopContainer = stopButton.closest('.flex.items-center.gap-3')
      const takeControlContainer = takeControlButton.closest('.flex.items-center.gap-3')
      expect(stopContainer).not.toBeNull()
      expect(stopContainer).toBe(takeControlContainer)
    })
  })
})
