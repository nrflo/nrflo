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

describe('AgentDefCard - restart_threshold display', () => {
  describe('restart threshold badge', () => {
    it('shows restart threshold badge when restart_threshold is set', () => {
      renderCard(makeAgentDef({ restart_threshold: 30 }))

      expect(screen.getByText('30% restart')).toBeInTheDocument()
    })

    it('shows restart threshold = 25 (global default)', () => {
      renderCard(makeAgentDef({ restart_threshold: 25 }))

      expect(screen.getByText('25% restart')).toBeInTheDocument()
    })

    it('shows restart threshold = 1 (minimum)', () => {
      renderCard(makeAgentDef({ restart_threshold: 1 }))

      expect(screen.getByText('1% restart')).toBeInTheDocument()
    })

    it('shows restart threshold = 99 (maximum)', () => {
      renderCard(makeAgentDef({ restart_threshold: 99 }))

      expect(screen.getByText('99% restart')).toBeInTheDocument()
    })

    it('does not show restart threshold badge when restart_threshold is undefined', () => {
      renderCard(makeAgentDef({ restart_threshold: undefined }))

      expect(screen.queryByText(/% restart/)).not.toBeInTheDocument()
    })

    it('does not show restart threshold badge when restart_threshold is null', () => {
      renderCard(makeAgentDef({ restart_threshold: null as any }))

      expect(screen.queryByText(/% restart/)).not.toBeInTheDocument()
    })
  })

  describe('badge positioning and layout', () => {
    it('shows restart threshold alongside model and timeout', () => {
      renderCard(makeAgentDef({ restart_threshold: 30, model: 'opus', timeout: 15 }))

      const container = screen.getByText('30% restart').closest('div')
      expect(container).toBeInTheDocument()

      // Verify model and timeout are also present
      expect(screen.getByText('opus')).toBeInTheDocument()
      expect(screen.getByText('15m timeout')).toBeInTheDocument()
    })

    it('displays all metadata in same row', () => {
      renderCard(makeAgentDef({ restart_threshold: 35 }))

      const agentId = screen.getByText('test-agent')
      const model = screen.getByText('sonnet')
      const timeout = screen.getByText('20m timeout')
      const restart = screen.getByText('35% restart')

      // All elements should be in the same parent container
      const parent = agentId.closest('.flex')
      expect(parent).toContainElement(model)
      expect(parent).toContainElement(timeout)
      expect(parent).toContainElement(restart)
    })
  })

  describe('styling and appearance', () => {
    it('renders restart threshold as text with muted styling', () => {
      renderCard(makeAgentDef({ restart_threshold: 40 }))

      const restart = screen.getByText('40% restart')
      expect(restart).toHaveClass('text-xs', 'text-muted-foreground')
    })

    it('matches timeout styling pattern', () => {
      renderCard(makeAgentDef({ restart_threshold: 30 }))

      const timeout = screen.getByText('20m timeout')
      const restart = screen.getByText('30% restart')

      // Both should have same classes (text-xs text-muted-foreground)
      const timeoutClasses = Array.from(timeout.classList)
      const restartClasses = Array.from(restart.classList)

      expect(timeoutClasses).toContain('text-xs')
      expect(restartClasses).toContain('text-xs')
      expect(timeoutClasses).toContain('text-muted-foreground')
      expect(restartClasses).toContain('text-muted-foreground')
    })
  })

  describe('edit form interaction', () => {
    it('edit form receives restart_threshold from initial data', async () => {
      const { container } = renderCard(makeAgentDef({ restart_threshold: 45 }))

      // Click edit button to open form
      const editButton = screen.getByTitle('Edit')
      await editButton.click()

      // Form should render with restart_threshold input
      const input = container.querySelector('input[type="number"][placeholder="25"]')
      expect(input).toBeInTheDocument()
      expect(input).toHaveValue(45)
    })
  })

  describe('multiple agent cards', () => {
    it('each card shows its own restart_threshold independently', () => {
      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      render(
        <QueryClientProvider client={queryClient}>
          <div>
            <AgentDefCard def={makeAgentDef({ id: 'agent-1', restart_threshold: 20 })} workflowId="feature" groups={[]} />
            <AgentDefCard def={makeAgentDef({ id: 'agent-2', restart_threshold: 40 })} workflowId="feature" groups={[]} />
            <AgentDefCard def={makeAgentDef({ id: 'agent-3', restart_threshold: undefined })} workflowId="feature" groups={[]} />
          </div>
        </QueryClientProvider>
      )

      expect(screen.getByText('20% restart')).toBeInTheDocument()
      expect(screen.getByText('40% restart')).toBeInTheDocument()
      expect(screen.queryByText(/agent-3/)).toBeInTheDocument()

      // agent-3 should not have restart badge
      const agent3Card = screen.getByText('agent-3').closest('div')
      expect(agent3Card).not.toHaveTextContent('% restart')
    })
  })
})
