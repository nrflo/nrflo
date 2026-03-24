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

describe('AgentDefCard - low consumption badge', () => {
  it('shows "alt: <id>" badge when low_consumption_agent is set', () => {
    renderCard(makeAgentDef({ low_consumption_agent: 'haiku-agent' }))
    expect(screen.getByText('alt: haiku-agent')).toBeInTheDocument()
  })

  it('does not show badge when low_consumption_agent is undefined', () => {
    renderCard(makeAgentDef({ low_consumption_agent: undefined }))
    expect(screen.queryByText(/^alt:/)).not.toBeInTheDocument()
  })

  it('does not show badge when low_consumption_agent is empty string', () => {
    renderCard(makeAgentDef({ low_consumption_agent: '' }))
    expect(screen.queryByText(/^alt:/)).not.toBeInTheDocument()
  })

  it('badge text uses the actual agent id', () => {
    renderCard(makeAgentDef({ low_consumption_agent: 'fast-haiku' }))
    expect(screen.getByText('alt: fast-haiku')).toBeInTheDocument()
  })

  it('badge appears alongside model badge', () => {
    renderCard(makeAgentDef({ model: 'opus', low_consumption_agent: 'haiku-agent' }))
    expect(screen.getByText('alt: haiku-agent')).toBeInTheDocument()
    expect(screen.getByText('opus')).toBeInTheDocument()
  })
})
