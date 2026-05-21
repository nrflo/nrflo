import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { AgentDefsSection } from './AgentDefsSection'
import { listAgentDefs } from '@/api/agentDefs'
import { setLayerPolicy } from '@/api/workflowLayerPolicies'
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
  listLayerPolicies: vi.fn().mockResolvedValue({ layer_policies: {}, layer_pause: {} }),
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

describe('AgentDefsSection - Pause after toggle', () => {
  it('renders Pause after toggle for a single-agent layer', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([makeAgent()])
    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)

    await screen.findByTestId('agent-def-card')
    expect(screen.getByText('Pause after')).toBeInTheDocument()
  })

  it('renders Pause after toggle for a multi-agent layer', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([
      makeAgent({ id: 'a1' }),
      makeAgent({ id: 'a2' }),
    ])
    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)

    await screen.findAllByTestId('agent-def-card')
    expect(screen.getByText('Pause after')).toBeInTheDocument()
  })

  it('toggle is unchecked by default (pause_after=false)', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([makeAgent()])

    const { listLayerPolicies } = await import('@/api/workflowLayerPolicies')
    vi.mocked(listLayerPolicies).mockResolvedValueOnce({ layer_policies: {}, layer_pause: {} })

    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)
    await screen.findByTestId('agent-def-card')

    const toggle = screen.getByRole('switch')
    expect(toggle).not.toBeChecked()
  })

  it('toggle is checked when layer_pause has true for that layer', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([makeAgent({ layer: 0 })])

    const { listLayerPolicies } = await import('@/api/workflowLayerPolicies')
    vi.mocked(listLayerPolicies).mockResolvedValueOnce({ layer_policies: {}, layer_pause: { 0: true } })

    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)
    await screen.findByTestId('agent-def-card')

    const toggle = screen.getByRole('switch')
    expect(toggle).toBeChecked()
  })

  it('calls setLayerPolicy with pauseAfter=true when toggle is clicked on', async () => {
    const user = userEvent.setup()
    vi.mocked(listAgentDefs).mockResolvedValue([makeAgent()])

    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)
    await screen.findByTestId('agent-def-card')

    const toggle = screen.getByRole('switch')
    await user.click(toggle)

    expect(vi.mocked(setLayerPolicy)).toHaveBeenCalledWith('wf-1', 0, 'any', true)
  })

  it('calls setLayerPolicy with pauseAfter=false when toggle is clicked off', async () => {
    const user = userEvent.setup()
    vi.mocked(listAgentDefs).mockResolvedValue([makeAgent({ layer: 0 })])

    const { listLayerPolicies } = await import('@/api/workflowLayerPolicies')
    vi.mocked(listLayerPolicies).mockResolvedValueOnce({ layer_policies: {}, layer_pause: { 0: true } })

    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)
    await screen.findByTestId('agent-def-card')

    const toggle = screen.getByRole('switch')
    expect(toggle).toBeChecked()
    await user.click(toggle)

    expect(vi.mocked(setLayerPolicy)).toHaveBeenCalledWith('wf-1', 0, 'any', false)
  })

  it('renders one toggle per layer for multiple layers', async () => {
    vi.mocked(listAgentDefs).mockResolvedValue([
      makeAgent({ id: 'l0a', layer: 0 }),
      makeAgent({ id: 'l1a', layer: 1 }),
    ])
    renderWithQuery(<AgentDefsSection workflowId="wf-1" groups={[]} />)

    await screen.findAllByTestId('agent-def-card')
    // One "Pause after" label per layer
    expect(screen.getAllByText('Pause after')).toHaveLength(2)
  })
})
