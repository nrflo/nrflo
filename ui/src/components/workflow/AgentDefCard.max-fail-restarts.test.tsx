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
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentDefCard def={def} workflowId="feature" groups={[]} />
    </QueryClientProvider>
  )
}

describe('AgentDefCard - max_fail_restarts display', () => {
  describe('fail retry badge', () => {
    it('shows badge when max_fail_restarts > 0', () => {
      renderCard(makeAgentDef({ max_fail_restarts: 3 }))
      expect(screen.getByText('3x fail retry')).toBeInTheDocument()
    })

    it('shows badge for max_fail_restarts = 1', () => {
      renderCard(makeAgentDef({ max_fail_restarts: 1 }))
      expect(screen.getByText('1x fail retry')).toBeInTheDocument()
    })

    it('shows badge for max_fail_restarts = 10 (maximum)', () => {
      renderCard(makeAgentDef({ max_fail_restarts: 10 }))
      expect(screen.getByText('10x fail retry')).toBeInTheDocument()
    })

    it('does not show badge when max_fail_restarts is 0', () => {
      renderCard(makeAgentDef({ max_fail_restarts: 0 }))
      expect(screen.queryByText(/fail retry/)).not.toBeInTheDocument()
    })

    it('does not show badge when max_fail_restarts is undefined', () => {
      renderCard(makeAgentDef({ max_fail_restarts: undefined }))
      expect(screen.queryByText(/fail retry/)).not.toBeInTheDocument()
    })

    it('does not show badge when max_fail_restarts is null', () => {
      renderCard(makeAgentDef({ max_fail_restarts: null as any }))
      expect(screen.queryByText(/fail retry/)).not.toBeInTheDocument()
    })
  })

  describe('badge coexistence', () => {
    it('shows fail retry badge alongside restart_threshold badge', () => {
      renderCard(makeAgentDef({ restart_threshold: 30, max_fail_restarts: 2 }))
      expect(screen.getByText('30% restart')).toBeInTheDocument()
      expect(screen.getByText('2x fail retry')).toBeInTheDocument()
    })

    it('shows only restart_threshold badge when max_fail_restarts is 0', () => {
      renderCard(makeAgentDef({ restart_threshold: 25, max_fail_restarts: 0 }))
      expect(screen.getByText('25% restart')).toBeInTheDocument()
      expect(screen.queryByText(/fail retry/)).not.toBeInTheDocument()
    })
  })

  describe('edit form pre-fill', () => {
    it('edit form receives max_fail_restarts from initial data', async () => {
      const { container } = renderCard(makeAgentDef({ max_fail_restarts: 4 }))

      const editButton = screen.getByTitle('Edit')
      await editButton.click()

      const input = container.querySelector('input[type="number"][placeholder="0"]')
      expect(input).toBeInTheDocument()
      expect(input).toHaveValue(4)
    })
  })
})
