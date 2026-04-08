import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WorkflowsPage } from './WorkflowsPage'
import * as workflowsApi from '@/api/workflows'
import type { WorkflowDefSummary, PhaseDef } from '@/types/workflow'

// Mock the workflows API
vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
  createWorkflowDef: vi.fn(),
  updateWorkflowDef: vi.fn(),
  deleteWorkflowDef: vi.fn(),
}))

// Mock project store
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: () => 'test-project',
}))

// Mock AgentDefsSection to avoid deep dependencies
vi.mock('@/components/workflow/AgentDefsSection', () => ({
  AgentDefsSection: () => <div data-testid="agent-defs-section">Agent Definitions</div>,
}))

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <WorkflowsPage />
    </QueryClientProvider>
  )
}

function mockWorkflowDefs(defs: Record<string, WorkflowDefSummary>) {
  vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue(defs)
}

describe('WorkflowsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('pipeline display with layer brackets', () => {
    it('displays single agent per layer without brackets', async () => {
      const defs: Record<string, WorkflowDefSummary> = {
        bugfix: {
          description: 'Bug fix workflow',
          phases: [
            { id: 'setup-analyzer', agent: 'setup-analyzer', layer: 0 },
            { id: 'implementor', agent: 'implementor', layer: 1 },
            { id: 'qa-verifier', agent: 'qa-verifier', layer: 2 },
          ],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('bugfix')).toBeInTheDocument()
      })

      // Expand the workflow card
      const expandButton = screen.getByRole('button', { name: /bugfix/i })
      await userEvent.click(expandButton)

      // Pipeline should show: "setup-analyzer -> implementor -> qa-verifier"
      await waitFor(() => {
        expect(screen.getByText('setup-analyzer -> implementor -> qa-verifier')).toBeInTheDocument()
      })
    })

    it('displays brackets for same-layer agents like [a | b]', async () => {
      const defs: Record<string, WorkflowDefSummary> = {
        feature: {
          description: 'Feature workflow',
          phases: [
            { id: 'analyzer-a', agent: 'analyzer-a', layer: 0 },
            { id: 'analyzer-b', agent: 'analyzer-b', layer: 0 },
            { id: 'implementor', agent: 'implementor', layer: 1 },
          ],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('feature')).toBeInTheDocument()
      })

      const expandButton = screen.getByRole('button', { name: /feature/i })
      await userEvent.click(expandButton)

      // Pipeline should show: "[analyzer-a | analyzer-b] -> implementor"
      await waitFor(() => {
        expect(screen.getByText('[analyzer-a | analyzer-b] -> implementor')).toBeInTheDocument()
      })
    })

    it('handles multiple parallel layers correctly', async () => {
      const defs: Record<string, WorkflowDefSummary> = {
        complex: {
          description: 'Complex workflow',
          phases: [
            { id: 'a1', agent: 'a1', layer: 0 },
            { id: 'a2', agent: 'a2', layer: 0 },
            { id: 'b', agent: 'b', layer: 1 },
            { id: 'c1', agent: 'c1', layer: 2 },
            { id: 'c2', agent: 'c2', layer: 2 },
            { id: 'c3', agent: 'c3', layer: 2 },
            { id: 'd', agent: 'd', layer: 3 },
          ],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('complex')).toBeInTheDocument()
      })

      const expandButton = screen.getByRole('button', { name: /complex/i })
      await userEvent.click(expandButton)

      // Pipeline: "[a1 | a2] -> b -> [c1 | c2 | c3] -> d"
      await waitFor(() => {
        expect(screen.getByText('[a1 | a2] -> b -> [c1 | c2 | c3] -> d')).toBeInTheDocument()
      })
    })

    it('displays all agents in layer 0 with brackets', async () => {
      const defs: Record<string, WorkflowDefSummary> = {
        parallel: {
          description: 'All parallel',
          phases: [
            { id: 'agent-a', agent: 'agent-a', layer: 0 },
            { id: 'agent-b', agent: 'agent-b', layer: 0 },
            { id: 'agent-c', agent: 'agent-c', layer: 0 },
          ],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('parallel')).toBeInTheDocument()
      })

      const expandButton = screen.getByRole('button', { name: /parallel/i })
      await userEvent.click(expandButton)

      await waitFor(() => {
        expect(screen.getByText('[agent-a | agent-b | agent-c]')).toBeInTheDocument()
      })
    })

    it('handles empty phases array gracefully', async () => {
      const defs: Record<string, WorkflowDefSummary> = {
        empty: {
          description: 'Empty workflow',
          phases: [],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('empty')).toBeInTheDocument()
      })

      const expandButton = screen.getByRole('button', { name: /empty/i })
      await userEvent.click(expandButton)

      // With no phases, pipeline section is not rendered
      await waitFor(() => {
        expect(screen.queryByText(/agent pipeline/i)).not.toBeInTheDocument()
      })
    })

    it('shows agent count in collapsed card', async () => {
      const defs: Record<string, WorkflowDefSummary> = {
        feature: {
          description: 'Feature workflow',
          phases: [
            { id: 'a', agent: 'a', layer: 0 },
            { id: 'b', agent: 'b', layer: 0 },
            { id: 'c', agent: 'c', layer: 1 },
          ],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('3 agents')).toBeInTheDocument()
      })
    })

    it('handles missing layer field gracefully (defaults to 0)', async () => {
      const defs: Record<string, WorkflowDefSummary> = {
        legacy: {
          description: 'Legacy workflow',
          phases: [
            { id: 'a', agent: 'a' } as PhaseDef, // Missing layer
            { id: 'b', agent: 'b', layer: 1 },
          ],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('legacy')).toBeInTheDocument()
      })

      const expandButton = screen.getByRole('button', { name: /legacy/i })
      await userEvent.click(expandButton)

      // Should treat missing layer as 0
      await waitFor(() => {
        expect(screen.getByText('a -> b')).toBeInTheDocument()
      })
    })

    it('sorts agents within same layer by original order', async () => {
      const defs: Record<string, WorkflowDefSummary> = {
        ordered: {
          description: 'Ordered workflow',
          phases: [
            { id: 'z-agent', agent: 'z-agent', layer: 0 },
            { id: 'a-agent', agent: 'a-agent', layer: 0 },
            { id: 'm-agent', agent: 'm-agent', layer: 0 },
          ],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('ordered')).toBeInTheDocument()
      })

      const expandButton = screen.getByRole('button', { name: /ordered/i })
      await userEvent.click(expandButton)

      // Should preserve order: z, a, m (not alphabetically sorted)
      await waitFor(() => {
        expect(screen.getByText('[z-agent | a-agent | m-agent]')).toBeInTheDocument()
      })
    })
  })

  describe('workflow list rendering', () => {
    it('shows loading state initially', () => {
      mockWorkflowDefs({})
      renderPage()

      expect(screen.getByText(/loading workflows/i)).toBeInTheDocument()
    })

    it('shows empty state when no workflows exist', async () => {
      mockWorkflowDefs({})
      renderPage()

      await waitFor(() => {
        expect(screen.getByText(/no workflow definitions found/i)).toBeInTheDocument()
      })
    })

    it('renders workflow cards for each definition', async () => {
      const defs: Record<string, WorkflowDefSummary> = {
        feature: {
          description: 'Feature workflow',
          phases: [{ id: 'setup-analyzer', agent: 'setup-analyzer', layer: 0 }],
        },
        bugfix: {
          description: 'Bug fix workflow',
          phases: [{ id: 'implementor', agent: 'implementor', layer: 0 }],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('feature')).toBeInTheDocument()
        expect(screen.getByText('bugfix')).toBeInTheDocument()
      })
    })

    it('shows description when provided', async () => {
      const defs: Record<string, WorkflowDefSummary> = {
        feature: {
          description: 'Full TDD workflow with all phases',
          phases: [],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('Full TDD workflow with all phases')).toBeInTheDocument()
      })
    })
  })

  describe('workflow card expansion', () => {
    it('expands to show pipeline and agent defs when clicked', async () => {
      const user = userEvent.setup()
      const defs: Record<string, WorkflowDefSummary> = {
        feature: {
          description: 'Feature workflow',
          phases: [{ id: 'setup-analyzer', agent: 'setup-analyzer', layer: 0 }],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('feature')).toBeInTheDocument()
      })

      const expandButton = screen.getByRole('button', { name: /feature/i })
      await user.click(expandButton)

      await waitFor(() => {
        expect(screen.getByText('Agent Pipeline')).toBeInTheDocument()
        expect(screen.getByTestId('agent-defs-section')).toBeInTheDocument()
      })
    })

    it('collapses when clicked again', async () => {
      const user = userEvent.setup()
      const defs: Record<string, WorkflowDefSummary> = {
        feature: {
          description: 'Feature workflow',
          phases: [{ id: 'setup-analyzer', agent: 'setup-analyzer', layer: 0 }],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('feature')).toBeInTheDocument()
      })

      const expandButton = screen.getByRole('button', { name: /feature/i })
      await user.click(expandButton)

      await waitFor(() => {
        expect(screen.getByText('Agent Pipeline')).toBeInTheDocument()
      })

      await user.click(expandButton)

      await waitFor(() => {
        expect(screen.queryByText('Agent Pipeline')).not.toBeInTheDocument()
      })
    })
  })

  describe('workflow actions', () => {
    it('opens create dialog when create button clicked', async () => {
      const user = userEvent.setup()
      mockWorkflowDefs({})
      renderPage()

      const createButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(createButton)

      // Dialog renders a heading "Create Workflow" — use heading role to distinguish from button
      await waitFor(() => {
        expect(screen.getByRole('heading', { name: /create workflow/i })).toBeInTheDocument()
      })
    })

    it('opens edit dialog when edit button clicked', async () => {
      const user = userEvent.setup()
      const defs: Record<string, WorkflowDefSummary> = {
        feature: {
          description: 'Feature workflow',
          phases: [],
        },
      }
      mockWorkflowDefs(defs)
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('feature')).toBeInTheDocument()
      })

      const editButton = screen.getByTitle('Edit workflow')
      await user.click(editButton)

      await waitFor(() => {
        expect(screen.getByText('Edit Workflow: feature')).toBeInTheDocument()
      })
    })

    it('prompts confirmation and deletes workflow when delete clicked', async () => {
      const user = userEvent.setup()
      const defs: Record<string, WorkflowDefSummary> = {
        feature: {
          description: 'Feature workflow',
          phases: [],
        },
      }
      mockWorkflowDefs(defs)
      vi.mocked(workflowsApi.deleteWorkflowDef).mockResolvedValue({ status: 'ok' })

      renderPage()

      await waitFor(() => {
        expect(screen.getByText('feature')).toBeInTheDocument()
      })

      const deleteButton = screen.getByTitle('Delete workflow')
      await user.click(deleteButton)

      const confirmButton = await screen.findByRole('button', { name: 'Delete' })
      await user.click(confirmButton)

      await waitFor(() => {
        expect(workflowsApi.deleteWorkflowDef).toHaveBeenCalledWith('feature')
      })
    })

    it('does not delete when confirmation is cancelled', async () => {
      const user = userEvent.setup()
      const defs: Record<string, WorkflowDefSummary> = {
        feature: {
          description: 'Feature workflow',
          phases: [],
        },
      }
      mockWorkflowDefs(defs)

      renderPage()

      await waitFor(() => {
        expect(screen.getByText('feature')).toBeInTheDocument()
      })

      const deleteButton = screen.getByTitle('Delete workflow')
      await user.click(deleteButton)

      const cancelButton = await screen.findByRole('button', { name: 'Cancel' })
      await user.click(cancelButton)

      expect(workflowsApi.deleteWorkflowDef).not.toHaveBeenCalled()
    })
  })

  describe('error handling', () => {
    it('shows error message when workflow loading fails', async () => {
      vi.mocked(workflowsApi.listWorkflowDefs).mockRejectedValue(
        new Error('Network error')
      )
      renderPage()

      await waitFor(() => {
        expect(screen.getByText(/failed to load workflows/i)).toBeInTheDocument()
      })
    })
  })

  describe('full user flow', () => {
    it('creates a workflow without phases (layers managed per-agent)', async () => {
      const user = userEvent.setup()
      mockWorkflowDefs({})
      vi.mocked(workflowsApi.createWorkflowDef).mockResolvedValue({
        id: 'test-workflow',
        project_id: 'test-project',
        description: 'Test workflow',
        phases: [],
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      })

      renderPage()

      // Open create dialog
      const createButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(createButton)

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: /create workflow/i })).toBeInTheDocument()
      })

      // Fill in workflow ID
      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'test-workflow')

      // Submit — the form has a submit button with text "Create Workflow"
      // which is different from the page button (now behind the dialog)
      const submitButton = screen.getAllByRole('button', { name: /create workflow/i })[1]
      await user.click(submitButton)

      await waitFor(() => {
        expect(workflowsApi.createWorkflowDef).toHaveBeenCalledWith(
          expect.objectContaining({
            id: 'test-workflow',
          })
        )
      })

      // Verify no phases field in the create request
      const callArgs = vi.mocked(workflowsApi.createWorkflowDef).mock.calls[0][0]
      expect(callArgs).not.toHaveProperty('phases')
    })
  })
})
