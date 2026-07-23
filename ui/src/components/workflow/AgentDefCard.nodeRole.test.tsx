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
    layer: 0,
    model: 'sonnet',
    timeout: 20,
    prompt: 'Test prompt',
    execution_mode: 'cli_interactive',
    tools: '',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderCard(def: AgentDef) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentDefCard def={def} workflowId="feature" groups={[]} />
    </QueryClientProvider>
  )
}

describe('AgentDefCard — description and node-role badge', () => {
  it('renders the description text when present', () => {
    renderCard(makeAgentDef({ description: 'Investigates the codebase before implementation' }))
    expect(screen.getByText('Investigates the codebase before implementation')).toBeInTheDocument()
  })

  it('renders nothing extra when description is absent', () => {
    renderCard(makeAgentDef({ description: undefined }))
    expect(screen.queryByText(/investigates/i)).not.toBeInTheDocument()
  })

  it('shows a "Fanout template" badge for node_role=fanout_template', () => {
    renderCard(makeAgentDef({ node_role: 'fanout_template', description: 'desc' }))
    expect(screen.getByText('Fanout template')).toBeInTheDocument()
  })

  it('shows a "Planner" badge for node_role=planner', () => {
    renderCard(makeAgentDef({ node_role: 'planner' }))
    expect(screen.getByText('Planner')).toBeInTheDocument()
  })

  it('node-role badge has amber styling', () => {
    renderCard(makeAgentDef({ node_role: 'fanout_template', description: 'desc' }))
    const badge = screen.getByText('Fanout template')
    expect(badge.className).toContain('border-amber-300')
    expect(badge.className).toContain('text-amber-600')
  })

  it('does not show a node-role badge for node_role=static', () => {
    renderCard(makeAgentDef({ node_role: 'static' }))
    expect(screen.queryByText('Fanout template')).not.toBeInTheDocument()
    expect(screen.queryByText('Planner')).not.toBeInTheDocument()
  })

  it('does not show a node-role badge when node_role is undefined', () => {
    renderCard(makeAgentDef({ node_role: undefined }))
    expect(screen.queryByText('Fanout template')).not.toBeInTheDocument()
    expect(screen.queryByText('Planner')).not.toBeInTheDocument()
  })
})
