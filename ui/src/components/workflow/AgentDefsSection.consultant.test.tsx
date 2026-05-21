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

describe('AgentDefsSection - consultant grouping', () => {
  it('renders Consultants heading when consultant defs exist', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([
      makeAgent({ id: 'phase-agent', consultant: false }),
      makeAgent({ id: 'sec-expert', consultant: true, execution_mode: 'api' }),
    ])
    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)
    expect(await screen.findByText('Consultants')).toBeInTheDocument()
  })

  it('does not render Consultants heading when no consultant defs', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([
      makeAgent({ id: 'phase-agent', consultant: false }),
    ])
    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)
    await screen.findByText('phase-agent')
    expect(screen.queryByText('Consultants')).not.toBeInTheDocument()
  })

  it('consultant def appears in Consultants group, not in phase layers', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([
      makeAgent({ id: 'phase-agent', layer: 0, consultant: false }),
      makeAgent({ id: 'sec-expert', layer: 0, consultant: true, execution_mode: 'api' }),
    ])
    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)

    await screen.findByText('sec-expert')
    // Phase layer row has "Layer 0" label; consultant group does not
    const layerLabel = screen.getByText(/Layer 0/i)
    // sec-expert should NOT be inside the layer group
    expect(layerLabel.closest('div')?.textContent).not.toContain('sec-expert')
    // Both cards are rendered
    const cards = screen.getAllByTestId('agent-def-card')
    expect(cards).toHaveLength(2)
  })

  it('consultant defs do not contribute to phase layer groups', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([
      makeAgent({ id: 'c1', consultant: true, execution_mode: 'api', layer: 0 }),
      makeAgent({ id: 'c2', consultant: true, execution_mode: 'api', layer: 0 }),
    ])
    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)

    await screen.findByText('Consultants')
    // No phase layer label since all defs are consultants
    expect(screen.queryByText(/Layer 0/i)).not.toBeInTheDocument()
  })
})
