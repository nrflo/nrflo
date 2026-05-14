import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import { AgentDefsSection } from './AgentDefsSection'
import { listAgentDefs } from '@/api/agentDefs'
import type { AgentDef } from '@/types/workflow'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (sel: (s: { currentProject: string }) => string) =>
    sel({ currentProject: 'proj-1' }),
}))

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn(),
  createAgentDef: vi.fn(),
}))

vi.mock('@/api/workflowLayerPolicies', () => ({
  listLayerPolicies: vi.fn().mockResolvedValue({}),
  setLayerPolicy: vi.fn().mockResolvedValue({ status: 'ok' }),
  deleteLayerPolicy: vi.fn().mockResolvedValue({ status: 'ok' }),
}))

vi.mock('@/components/workflow/AgentDefCard', () => ({
  AgentDefCard: ({ def }: { def: AgentDef }) => (
    <div data-testid="agent-def-card">{def.id}</div>
  ),
}))

vi.mock('@/components/workflow/AgentDefForm', () => ({
  AgentDefForm: () => <div data-testid="agent-def-form" />,
}))

function makeAgent(overrides: Partial<AgentDef> = {}): AgentDef {
  return {
    id: 'agent-1',
    project_id: 'proj-1',
    workflow_id: 'wf-1',
    layer: 0,
    model: 'sonnet',
    timeout: 600,
    prompt: 'test prompt',
    execution_mode: 'cli_interactive',
    tools: '',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('AgentDefsSection - layer policy display', () => {
  it('renders LayerPolicyControl with Dropdown for a layer with 2 agents', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([
      makeAgent({ id: 'a1' }),
      makeAgent({ id: 'a2' }),
    ])
    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)

    // LayerPolicyControl renders "Layer {layer} policy:" label
    expect(await screen.findByText(/Layer 0 policy:/i)).toBeInTheDocument()
    // Dropdown trigger with default "any" policy
    const btn = screen.getByText(/Layer 0 policy:/i).parentElement!.querySelector(
      'button[type="button"]'
    ) as HTMLElement
    expect(btn.textContent).toContain('any (1 agent must pass)')
  })

  it('shows existing policy in Dropdown when layer_policies has a value', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([
      makeAgent({ id: 'a1' }),
      makeAgent({ id: 'a2' }),
    ])
    const { listLayerPolicies } = await import('@/api/workflowLayerPolicies')
    vi.mocked(listLayerPolicies).mockResolvedValueOnce({ 0: 'quorum:2' })

    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)

    await screen.findByText(/Layer 0 policy:/i)
    const btn = screen.getByText(/Layer 0 policy:/i).parentElement!.querySelector(
      'button[type="button"]'
    ) as HTMLElement
    expect(btn.textContent).toContain('quorum (N agents must pass)')
  })

  it('shows static "any" Badge with no Dropdown for a single-agent layer', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([makeAgent({ id: 'a1' })])
    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)

    // Wait for agent card to appear
    await screen.findByTestId('agent-def-card')

    // Static badge shows "any"
    expect(screen.getByText('any')).toBeInTheDocument()
    // No LayerPolicyControl label
    expect(screen.queryByText(/Layer 0 policy:/i)).not.toBeInTheDocument()
  })

  it('renders two agent cards for a two-agent layer', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([
      makeAgent({ id: 'a1' }),
      makeAgent({ id: 'a2' }),
    ])
    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)

    const cards = await screen.findAllByTestId('agent-def-card')
    expect(cards).toHaveLength(2)
  })

  it('renders agents across multiple layers with correct per-layer controls', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([
      makeAgent({ id: 'l0a', layer: 0 }),
      makeAgent({ id: 'l1a', layer: 1 }),
      makeAgent({ id: 'l1b', layer: 1 }),
    ])
    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)

    // Wait for all agents to load
    await screen.findAllByTestId('agent-def-card')

    // Layer 0: single agent → static badge
    expect(screen.getByText(/Layer 0:/i)).toBeInTheDocument()
    expect(screen.queryByText(/Layer 0 policy:/i)).not.toBeInTheDocument()

    // Layer 1: two agents → LayerPolicyControl
    expect(screen.getByText(/Layer 1 policy:/i)).toBeInTheDocument()
  })
})
