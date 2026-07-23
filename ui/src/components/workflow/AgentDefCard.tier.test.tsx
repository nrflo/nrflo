import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import { AgentDefCard } from './AgentDefCard'
import { useTierModels } from '@/hooks/useTierModels'
import type { AgentDef } from '@/types/workflow'
import type { TierModel } from '@/api/tierModels'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn(() => 'test-project'),
}))

vi.mock('@/hooks/useTierModels', async () => {
  const actual = await vi.importActual<typeof import('@/hooks/useTierModels')>('@/hooks/useTierModels')
  return { ...actual, useTierModels: vi.fn() }
})

function makeRow(overrides: Partial<TierModel> = {}): TierModel {
  return {
    tier: 2,
    position: 0,
    provider: 'anthropic',
    execution_mode: 'cli_interactive',
    model_id: 'opus-4-8',
    reasoning_effort: '',
    ...overrides,
  }
}

function makeAgentDef(overrides: Partial<AgentDef> = {}): AgentDef {
  return {
    id: 'test-agent',
    project_id: 'test-project',
    workflow_id: 'feature',
    model: 'sonnet',
    timeout: 20,
    prompt: 'Test prompt',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderCard(def: AgentDef, groups: string[] = []) {
  return renderWithQuery(<AgentDefCard def={def} workflowId="feature" groups={groups} />)
}

describe('AgentDefCard - tier badges', () => {
  it('shows a Tier badge and the resolved chain-primary model for a tier def (model === "")', () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [makeRow({ tier: 2 })] } as any)
    renderCard(makeAgentDef({ model: '', tier: 2 }))

    expect(screen.getByText('Tier 2')).toBeInTheDocument()
    expect(screen.getByText('opus-4-8')).toBeInTheDocument()
  })

  it('shows a plain model badge for an override def (non-empty model)', () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [] } as any)
    renderCard(makeAgentDef({ model: 'sonnet-5', tier: null }))

    expect(screen.getByText('sonnet-5')).toBeInTheDocument()
    expect(screen.queryByText(/^Tier /)).not.toBeInTheDocument()
  })

  it('renders a graceful "—" when resolveTierChain returns []', () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [] } as any)
    renderCard(makeAgentDef({ model: '', tier: 5 }))

    expect(screen.getByText('Tier 5')).toBeInTheDocument()
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})
