import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { WorkflowsPage } from './WorkflowsPage'
import { listWorkflowDefs } from '@/api/workflows'
import type { WorkflowDefSummary } from '@/types/workflow'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (sel: (s: { currentProject: string }) => string) =>
    sel({ currentProject: 'proj-1' }),
}))

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
  createWorkflowDef: vi.fn(),
  updateWorkflowDef: vi.fn(),
  deleteWorkflowDef: vi.fn(),
}))

vi.mock('@/components/workflow/AgentDefsSection', () => ({
  AgentDefsSection: () => <div data-testid="agent-defs-section" />,
}))

vi.mock('@/components/workflow/WorkflowDefForm', () => ({
  WorkflowDefForm: () => <div data-testid="workflow-def-form" />,
}))

function makeSummary(overrides: Partial<WorkflowDefSummary> = {}): WorkflowDefSummary {
  return {
    description: 'Test workflow',
    phases: [],
    ...overrides,
  }
}

async function expandWorkflowCard(user: ReturnType<typeof userEvent.setup>, name: string) {
  // Click the workflow name text (inside the expand button)
  await user.click(await screen.findByText(name))
}

describe('WorkflowsPage - phase summary policy suffix', () => {
  it('appends ·quorum:2 suffix to multi-agent layer bracket when layer_policies is set', async () => {
    const user = userEvent.setup()
    vi.mocked(listWorkflowDefs).mockResolvedValue({
      'my-wf': makeSummary({
        phases: [
          { id: 'a1', agent: 'agent-a', layer: 0 },
          { id: 'a2', agent: 'agent-b', layer: 0 },
        ],
        layer_policies: { 0: 'quorum:2' },
      }),
    })

    renderWithQuery(<WorkflowsPage />)
    await expandWorkflowCard(user, 'my-wf')

    expect(screen.getByText('[agent-a | agent-b]·quorum:2')).toBeInTheDocument()
  })

  it('omits suffix when layer_policies is absent', async () => {
    const user = userEvent.setup()
    vi.mocked(listWorkflowDefs).mockResolvedValue({
      'my-wf': makeSummary({
        phases: [
          { id: 'a1', agent: 'agent-a', layer: 0 },
          { id: 'a2', agent: 'agent-b', layer: 0 },
        ],
      }),
    })

    renderWithQuery(<WorkflowsPage />)
    await expandWorkflowCard(user, 'my-wf')

    expect(screen.getByText('[agent-a | agent-b]')).toBeInTheDocument()
  })

  it('omits suffix when layer has only one agent', async () => {
    const user = userEvent.setup()
    vi.mocked(listWorkflowDefs).mockResolvedValue({
      'my-wf': makeSummary({
        phases: [{ id: 'a1', agent: 'solo-agent', layer: 0 }],
        layer_policies: { 0: 'quorum:1' },
      }),
    })

    renderWithQuery(<WorkflowsPage />)
    await expandWorkflowCard(user, 'my-wf')

    // Single-agent layer is shown without brackets or suffix
    expect(screen.getByText('solo-agent')).toBeInTheDocument()
    expect(screen.queryByText(/\[solo-agent\]/)).not.toBeInTheDocument()
  })

  it('shows multi-layer pipeline with policy suffix on the correct layer', async () => {
    const user = userEvent.setup()
    vi.mocked(listWorkflowDefs).mockResolvedValue({
      'my-wf': makeSummary({
        phases: [
          { id: 'a1', agent: 'setup', layer: 0 },
          { id: 'a2', agent: 'impl-a', layer: 1 },
          { id: 'a3', agent: 'impl-b', layer: 1 },
        ],
        layer_policies: { 1: 'all' },
      }),
    })

    renderWithQuery(<WorkflowsPage />)
    await expandWorkflowCard(user, 'my-wf')

    expect(screen.getByText('setup -> [impl-a | impl-b]·all')).toBeInTheDocument()
  })
})
