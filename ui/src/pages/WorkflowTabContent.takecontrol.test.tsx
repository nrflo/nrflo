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

function makeCodexAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a3',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'codex:gpt-5.6-luna',
    cli: 'codex',
    effective_mode: 'cli_interactive',
    pid: 54321,
    session_id: 'sess-codex-789',
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
    const visibilityCases: Array<[string, Record<string, unknown>, boolean]> = [
      ['shows when onTakeControl provided and Claude agent is running', {
        onTakeControl: vi.fn(), activeAgents: { a: makeClaudeAgent() }, hasActivePhase: true,
      }, true],
      ['hides when no onTakeControl prop', {
        activeAgents: { a: makeClaudeAgent() }, hasActivePhase: true, onTakeControl: undefined,
      }, false],
      ['hides when agent has no session_id', {
        onTakeControl: vi.fn(), activeAgents: { a: makeClaudeAgent({ session_id: undefined }) }, hasActivePhase: true,
      }, false],
      ['hides for an unknown cli value', {
        onTakeControl: vi.fn(), activeAgents: { a: makeGptAgent() }, hasActivePhase: true,
      }, false],
      ['shows for a running codex agent', {
        onTakeControl: vi.fn(), activeAgents: { a: makeCodexAgent() }, hasActivePhase: true,
      }, true],
      ['hides for an api-mode claude agent', {
        onTakeControl: vi.fn(), activeAgents: { a: makeClaudeAgent({ effective_mode: 'api' }) }, hasActivePhase: true,
      }, false],
      ['hides for a script-mode claude agent', {
        onTakeControl: vi.fn(), activeAgents: { a: makeClaudeAgent({ effective_mode: 'script' }) }, hasActivePhase: true,
      }, false],
      ['hides when agent has a result (completed)', {
        onTakeControl: vi.fn(), activeAgents: { a: makeClaudeAgent({ result: 'pass' }) }, hasActivePhase: false, isOrchestrated: false,
      }, false],
      ['hides when neither orchestrated nor hasActivePhase', {
        onTakeControl: vi.fn(), activeAgents: { a: makeClaudeAgent() }, hasActivePhase: false, isOrchestrated: false,
      }, false],
      ['shows when orchestrated (even with no active phase)', {
        onTakeControl: vi.fn(), activeAgents: { a: makeClaudeAgent() }, hasActivePhase: false, isOrchestrated: true,
      }, true],
    ]

    it.each(visibilityCases)('%s', (_name, overrides, expected) => {
      renderContent(overrides)
      const query = screen.queryByRole('button', { name: /take control/i })
      if (expected) {
        expect(query).toBeInTheDocument()
      } else {
        expect(query).not.toBeInTheDocument()
      }
    })
  })

  describe('disabled state', () => {
    it.each([
      ['disabled when takeControlPending is true', true, true],
      ['enabled when takeControlPending is false', false, false],
    ])('is %s', (_name, pending, disabled) => {
      renderContent({
        onTakeControl: vi.fn(),
        takeControlPending: pending,
        activeAgents: { a: makeClaudeAgent() },
        hasActivePhase: true,
      })

      const button = screen.getByRole('button', { name: /take control/i })
      if (disabled) {
        expect(button).toBeDisabled()
      } else {
        expect(button).not.toBeDisabled()
      }
    })
  })

  describe('click behavior', () => {
    it('calls onTakeControl with the running agent session_id when clicked', async () => {
      const user = userEvent.setup()
      const onTakeControl = vi.fn()

      renderContent({
        onTakeControl,
        activeAgents: { a: makeClaudeAgent({ session_id: 'my-session' }) },
        hasActivePhase: true,
      })

      await user.click(screen.getByRole('button', { name: /take control/i }))
      expect(onTakeControl).toHaveBeenCalledWith('my-session')
      expect(onTakeControl).toHaveBeenCalledTimes(1)
    })

    it('calls onTakeControl with the codex agent session_id when clicked', async () => {
      const user = userEvent.setup()
      const onTakeControl = vi.fn()

      renderContent({
        onTakeControl,
        activeAgents: { a: makeCodexAgent({ session_id: 'codex-session' }) },
        hasActivePhase: true,
      })

      await user.click(screen.getByRole('button', { name: /take control/i }))
      expect(onTakeControl).toHaveBeenCalledWith('codex-session')
    })

    it('prefers selectedPanelAgent session when it is a running Claude agent', async () => {
      const user = userEvent.setup()
      const onTakeControl = vi.fn()
      const panelAgent = makeClaudeAgent({ session_id: 'panel-session', agent_id: 'panel-1' })

      renderContent({
        onTakeControl,
        activeAgents: {
          a: makeClaudeAgent({ session_id: 'fallback-session' }),
          panel: panelAgent,
        },
        selectedPanelAgent: { agent: panelAgent, phaseName: 'implementation' },
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
        activeAgents: { a: makeClaudeAgent({ session_id: 'fallback-session' }) },
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
        activeAgents: { a: makeClaudeAgent({ session_id: 'running-session' }) },
        selectedPanelAgent: { agent: completedAgent, phaseName: 'implementation' },
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
        activeAgents: { a: makeClaudeAgent() },
        hasActivePhase: true,
      })

      const stopButton = screen.getByRole('button', { name: /stop/i })
      const takeControlButton = screen.getByRole('button', { name: /take control/i })
      const stopContainer = stopButton.closest('.flex.items-center.gap-3')
      const takeControlContainer = takeControlButton.closest('.flex.items-center.gap-3')
      expect(stopContainer).not.toBeNull()
      expect(stopContainer).toBe(takeControlContainer)
    })
  })
})
