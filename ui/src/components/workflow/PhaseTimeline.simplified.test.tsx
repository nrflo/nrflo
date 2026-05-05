import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PhaseTimeline } from './PhaseTimeline'
import { useGlobalSettings } from '@/hooks/useGlobalSettings'
import type { WorkflowState } from '@/types/workflow'

vi.mock('./PhaseGraph', () => ({
  PhaseGraph: () => <div data-testid="phase-graph">PhaseGraph</div>,
}))

vi.mock('./AgentsTable', () => ({
  AgentsTable: () => <div data-testid="agents-table">AgentsTable</div>,
}))

vi.mock('@/hooks/useTickets', () => ({
  useAgentSessions: vi.fn(() => ({ data: { sessions: [] }, isLoading: false })),
}))

vi.mock('@/hooks/useGlobalSettings', () => ({
  useGlobalSettings: vi.fn(() => ({ data: { simplified_agents_graph: false } })),
}))

function makeWorkflow(overrides: Partial<WorkflowState> = {}): WorkflowState {
  return {
    workflow: 'feature',
    version: 4,
    current_phase: 'implementation',
    status: 'active',
    phases: {
      investigation: { status: 'completed', result: 'pass' },
      implementation: { status: 'in_progress' },
    },
    phase_order: ['investigation', 'implementation'],
    findings: {},
    ...overrides,
  }
}

function renderPhaseTimeline(props: Partial<React.ComponentProps<typeof PhaseTimeline>> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <PhaseTimeline workflow={makeWorkflow()} {...props} />
    </QueryClientProvider>
  )
}

describe('PhaseTimeline simplified_agents_graph toggle', () => {
  it('renders AgentsTable and hides PhaseGraph when simplified_agents_graph is true', () => {
    vi.mocked(useGlobalSettings).mockReturnValue({ data: { simplified_agents_graph: true } } as any)
    renderPhaseTimeline()
    expect(screen.getByTestId('agents-table')).toBeInTheDocument()
    expect(screen.queryByTestId('phase-graph')).not.toBeInTheDocument()
  })

  it('renders PhaseGraph and hides AgentsTable when simplified_agents_graph is false', () => {
    vi.mocked(useGlobalSettings).mockReturnValue({ data: { simplified_agents_graph: false } } as any)
    renderPhaseTimeline()
    expect(screen.getByTestId('phase-graph')).toBeInTheDocument()
    expect(screen.queryByTestId('agents-table')).not.toBeInTheDocument()
  })

  it('renders PhaseGraph when global settings are not yet loaded', () => {
    vi.mocked(useGlobalSettings).mockReturnValue({ data: undefined } as any)
    renderPhaseTimeline()
    expect(screen.getByTestId('phase-graph')).toBeInTheDocument()
    expect(screen.queryByTestId('agents-table')).not.toBeInTheDocument()
  })
})
