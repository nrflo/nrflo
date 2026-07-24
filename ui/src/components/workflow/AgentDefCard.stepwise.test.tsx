import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentDefCard } from './AgentDefCard'
import type { AgentDef } from '@/types/workflow'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn(() => 'test-project'),
}))

vi.mock('@/api/tierModels', () => ({
  listTierModels: vi.fn().mockResolvedValue([]),
  setTierChain: vi.fn(),
}))

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
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentDefCard def={def} workflowId="feature" groups={groups} />
    </QueryClientProvider>
  )
}

describe('AgentDefCard - stepwise badge', () => {
  it('shows "stepwise · N steps" with the count parsed from def.steps', () => {
    renderCard(makeAgentDef({ prompt_mode: 'stepwise', steps: '[{"step_id":"a"},{"step_id":"b"}]' }))
    expect(screen.getByText('stepwise · 2 steps')).toBeInTheDocument()
  })

  it('is absent for a full-mode def', () => {
    renderCard(makeAgentDef({ prompt_mode: 'full', steps: '[{"step_id":"a"}]' }))
    expect(screen.queryByText(/stepwise ·/)).not.toBeInTheDocument()
  })

  it('is absent when prompt_mode is undefined', () => {
    renderCard(makeAgentDef())
    expect(screen.queryByText(/stepwise ·/)).not.toBeInTheDocument()
  })

  it('shows 0 steps when def.steps is missing but prompt_mode is stepwise', () => {
    renderCard(makeAgentDef({ prompt_mode: 'stepwise', steps: undefined }))
    expect(screen.getByText('stepwise · 0 steps')).toBeInTheDocument()
  })
})
