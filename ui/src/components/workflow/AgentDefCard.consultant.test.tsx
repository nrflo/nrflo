import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentDefCard } from './AgentDefCard'
import type { AgentDef } from '@/types/workflow'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn(() => 'test-project'),
}))

function makeAgentDef(overrides: Partial<AgentDef> = {}): AgentDef {
  return {
    id: 'test-agent',
    project_id: 'test-project',
    workflow_id: 'feature',
    model: 'sonnet',
    timeout: 20,
    prompt: 'Test prompt',
    execution_mode: 'cli_interactive',
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

describe('AgentDefCard — consultant badge', () => {
  it('shows Consultant badge when def.consultant is true', () => {
    renderCard(makeAgentDef({ consultant: true, execution_mode: 'api' }))
    expect(screen.getByText('Consultant')).toBeInTheDocument()
  })

  it('Consultant badge has indigo styling', () => {
    renderCard(makeAgentDef({ consultant: true, execution_mode: 'api' }))
    const badge = screen.getByText('Consultant')
    expect(badge.className).toContain('border-indigo-300')
    expect(badge.className).toContain('text-indigo-600')
  })

  it('does not show Consultant badge when def.consultant is false', () => {
    renderCard(makeAgentDef({ consultant: false }))
    expect(screen.queryByText('Consultant')).not.toBeInTheDocument()
  })

  it('does not show Consultant badge when def.consultant is undefined', () => {
    renderCard(makeAgentDef({ consultant: undefined }))
    expect(screen.queryByText('Consultant')).not.toBeInTheDocument()
  })

  it('API badge is suppressed for consultant defs', () => {
    renderCard(makeAgentDef({ consultant: true, execution_mode: 'api' }))
    expect(screen.queryByText('API')).not.toBeInTheDocument()
    expect(screen.getByText('Consultant')).toBeInTheDocument()
  })

  it('API badge shows for non-consultant api-mode defs', () => {
    renderCard(makeAgentDef({ consultant: false, execution_mode: 'api' }))
    expect(screen.getByText('API')).toBeInTheDocument()
    expect(screen.queryByText('Consultant')).not.toBeInTheDocument()
  })
})
