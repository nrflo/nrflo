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

function renderCard(def: AgentDef, groups: string[] = []) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentDefCard def={def} workflowId="feature" groups={groups} />
    </QueryClientProvider>
  )
}

describe('AgentDefCard - tag badge', () => {
  it('shows tag badge when tag is set', () => {
    renderCard(makeAgentDef({ tag: 'be' }), ['be', 'fe'])
    expect(screen.getByText('be')).toBeInTheDocument()
  })

  it('does not show tag badge when tag is undefined', () => {
    renderCard(makeAgentDef({ tag: undefined }))
    // no emerald badge should exist
    expect(document.querySelector('.border-emerald-300')).not.toBeInTheDocument()
  })

  it('does not show tag badge when tag is empty string', () => {
    renderCard(makeAgentDef({ tag: '' }))
    expect(document.querySelector('.border-emerald-300')).not.toBeInTheDocument()
  })

  it('tag badge has emerald styling', () => {
    renderCard(makeAgentDef({ tag: 'fe' }), ['be', 'fe'])
    expect(screen.getByText('fe')).toBeInTheDocument()
    expect(document.querySelector('.border-emerald-300')).toBeInTheDocument()
    expect(document.querySelector('.text-emerald-600')).toBeInTheDocument()
  })

  it('tag badge text matches the tag value', () => {
    renderCard(makeAgentDef({ tag: 'docs' }), ['be', 'docs'])
    expect(screen.getByText('docs')).toBeInTheDocument()
  })

  it('tag badge appears alongside model and timeout', () => {
    renderCard(makeAgentDef({ tag: 'be', model: 'opus', timeout: 15 }), ['be'])
    expect(screen.getByText('be')).toBeInTheDocument()
    expect(screen.getByText('opus')).toBeInTheDocument()
    expect(screen.getByText('15m timeout')).toBeInTheDocument()
  })
})
