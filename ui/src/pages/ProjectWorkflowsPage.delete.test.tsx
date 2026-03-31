import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ProjectWorkflowsPage } from './ProjectWorkflowsPage'
import type { WorkflowState } from '@/types/workflow'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projects: unknown[]; projectsLoaded: boolean }) => unknown) =>
    selector({
      currentProject: 'test-project',
      projects: [{ id: 'test-project', name: 'Test Project', root_path: '/test', default_branch: null, created_at: '', updated_at: '' }],
      projectsLoaded: true,
    }),
}))

vi.mock('@/hooks/useTickets', async () => {
  const actual = await vi.importActual<typeof import('@/hooks/useTickets')>('@/hooks/useTickets')
  return {
    ...actual,
    useProjectWorkflow: vi.fn(),
    useProjectAgentSessions: vi.fn(),
    useRunProjectWorkflow: vi.fn(),
    useStopProjectWorkflow: vi.fn(),
    useRetryFailedProjectAgent: vi.fn(),
    useDeleteProjectWorkflowInstance: vi.fn(),
  }
})

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn().mockResolvedValue({
    feature: { description: 'Feature workflow', scope_type: 'project', phases: [{ id: 'setup', agent: 'setup', layer: 0 }] },
    bugfix: { description: 'Bugfix workflow', scope_type: 'project', phases: [{ id: 'setup', agent: 'setup', layer: 0 }] },
  }),
}))

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn().mockResolvedValue([
    { id: 'setup', model: 'sonnet', timeout: 300, prompt: 'test', workflow_id: 'feature', project_id: 'test-project', created_at: '', updated_at: '' },
  ]),
}))

vi.mock('./WorkflowTabContent', () => ({
  WorkflowTabContent: (props: any) => (
    <div data-testid="workflow-tab-content">
      <div data-testid="displayed-status">{props.displayedState?.status}</div>
    </div>
  ),
}))

vi.mock('@/components/workflow/CompletedAgentsTable', () => ({
  CompletedAgentsTable: () => <div data-testid="completed-agents-table" />,
}))

vi.mock('@/components/workflow/AgentLogPanel', () => ({
  AgentLogPanel: () => <div data-testid="agent-log-panel" />,
}))

// --- Sample data (instance IDs are exactly 8 chars so shortId = full ID) ---

// "failinst" = 8 chars → label "feature (#failinst)"
const failedInstance: WorkflowState = {
  workflow: 'feature', instance_id: 'failinst', version: 4, scope_type: 'project',
  current_phase: 'implementation', status: 'failed',
  phases: { implementation: { status: 'failed', result: 'fail' } },
  phase_order: ['implementation'], active_agents: {}, agent_history: [], findings: {},
}

// "runinst1" = 8 chars → label "feature (#runinst1)"
const activeInstance: WorkflowState = {
  workflow: 'feature', instance_id: 'runinst1', version: 4, scope_type: 'project',
  current_phase: 'implementation', status: 'active',
  phases: { implementation: { status: 'in_progress' } },
  phase_order: ['implementation'], active_agents: {}, findings: {},
}

// "compinst" = 8 chars → label "bugfix (#compinst)"
const completedInstance: WorkflowState = {
  workflow: 'bugfix', instance_id: 'compinst', version: 4, scope_type: 'project',
  current_phase: 'verification', status: 'completed',
  phases: { verification: { status: 'completed', result: 'pass' } },
  phase_order: ['verification'], active_agents: {}, agent_history: [], findings: {},
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectWorkflowsPage />
    </QueryClientProvider>
  )
}

describe('ProjectWorkflowsPage — Failed tab and delete', () => {
  let useProjectWorkflow: any
  let useProjectAgentSessions: any
  let useRunProjectWorkflow: any
  let useStopProjectWorkflow: any
  let useRetryFailedProjectAgent: any
  let useDeleteProjectWorkflowInstance: any
  let deleteMutate: ReturnType<typeof vi.fn>

  beforeEach(async () => {
    const hooks = await import('@/hooks/useTickets')
    useProjectWorkflow = hooks.useProjectWorkflow as any
    useProjectAgentSessions = hooks.useProjectAgentSessions as any
    useRunProjectWorkflow = hooks.useRunProjectWorkflow as any
    useStopProjectWorkflow = hooks.useStopProjectWorkflow as any
    useRetryFailedProjectAgent = hooks.useRetryFailedProjectAgent as any
    useDeleteProjectWorkflowInstance = hooks.useDeleteProjectWorkflowInstance as any

    vi.clearAllMocks()
    deleteMutate = vi.fn()

    useProjectWorkflow.mockReturnValue({ data: undefined, isLoading: false })
    useProjectAgentSessions.mockReturnValue({ data: { project_id: 'test-project', sessions: [] }, isLoading: false })
    useRunProjectWorkflow.mockReturnValue({ mutateAsync: vi.fn(), isPending: false, isError: false, error: null })
    useStopProjectWorkflow.mockReturnValue({ mutate: vi.fn(), isPending: false })
    useRetryFailedProjectAgent.mockReturnValue({ mutate: vi.fn(), isPending: false })
    useDeleteProjectWorkflowInstance.mockReturnValue({ mutate: deleteMutate, isPending: false })
  })

  // --- Failed tab content ---

  describe('Failed tab', () => {
    it('renders WorkflowTabContent with failed status when Failed tab is clicked', async () => {
      const user = userEvent.setup()
      useProjectWorkflow.mockReturnValue({
        data: { project_id: 'test-project', has_workflow: true, state: failedInstance, workflows: ['feature'], all_workflows: { failinst: failedInstance } },
        isLoading: false,
      })

      renderPage()
      await user.click(screen.getByRole('button', { name: /Failed/ }))

      expect(screen.getByTestId('workflow-tab-content')).toBeInTheDocument()
      expect(screen.getByTestId('displayed-status').textContent).toBe('failed')
    })

    it('failed instance appears on Failed tab but NOT on Running tab', async () => {
      const user = userEvent.setup()
      useProjectWorkflow.mockReturnValue({
        data: { project_id: 'test-project', has_workflow: true, state: failedInstance, workflows: ['feature'], all_workflows: { failinst: failedInstance } },
        isLoading: false,
      })

      renderPage()

      // Running tab: 0 instances, no row for the failed instance
      await user.click(screen.getByRole('button', { name: /Running \(0\)/ }))
      expect(screen.queryByText('#failinst')).not.toBeInTheDocument()

      // Failed tab: 1 instance row visible
      await user.click(screen.getByRole('button', { name: /Failed \(1\)/ }))
      expect(screen.getByText('#failinst')).toBeInTheDocument()
    })
  })

  // --- Delete button visibility ---

  describe('Delete button in instance table', () => {
    it('shows trash icon in failed tab table row', async () => {
      const user = userEvent.setup()
      useProjectWorkflow.mockReturnValue({
        data: { project_id: 'test-project', has_workflow: true, state: failedInstance, workflows: ['feature'], all_workflows: { failinst: failedInstance } },
        isLoading: false,
      })

      renderPage()
      await user.click(screen.getByRole('button', { name: /Failed/ }))

      const row = screen.getByText('#failinst').closest('tr')!
      expect(within(row).getByRole('button')).toBeInTheDocument()
    })

    it('shows trash icon in completed tab table row', async () => {
      const user = userEvent.setup()
      useProjectWorkflow.mockReturnValue({
        data: { project_id: 'test-project', has_workflow: true, state: completedInstance, workflows: ['bugfix'], all_workflows: { compinst: completedInstance } },
        isLoading: false,
      })

      renderPage()
      await user.click(screen.getByRole('button', { name: /Completed/ }))

      const row = screen.getByText('#compinst').closest('tr')!
      expect(within(row).getByRole('button')).toBeInTheDocument()
    })

    it('does NOT show trash icon in running tab chip (no onDelete prop)', async () => {
      const user = userEvent.setup()
      useProjectWorkflow.mockReturnValue({
        data: { project_id: 'test-project', has_workflow: true, state: activeInstance, workflows: ['feature'], all_workflows: { runinst1: activeInstance } },
        isLoading: false,
      })

      renderPage()
      await user.click(screen.getByRole('button', { name: /Running/ }))

      const chip = screen.getByRole('button', { name: /#runinst1/ })
      // No nested role="button" (no trash icon for running tab)
      expect(within(chip).queryByRole('button')).not.toBeInTheDocument()
    })
  })

  // --- Delete confirmation flow ---

  describe('Delete confirmation flow', () => {
    it('opens ConfirmDialog with correct title when trash icon is clicked on failed instance', async () => {
      const user = userEvent.setup()
      useProjectWorkflow.mockReturnValue({
        data: { project_id: 'test-project', has_workflow: true, state: failedInstance, workflows: ['feature'], all_workflows: { failinst: failedInstance } },
        isLoading: false,
      })

      renderPage()
      await user.click(screen.getByRole('button', { name: /Failed/ }))

      const row = screen.getByText('#failinst').closest('tr')!
      await user.click(within(row).getByRole('button'))

      expect(await screen.findByText('Delete Workflow Instance')).toBeInTheDocument()
      expect(screen.getByText(/Are you sure you want to delete this workflow instance/)).toBeInTheDocument()
    })

    it('calls deleteMutation.mutate with correct projectId and instanceId when confirmed', async () => {
      const user = userEvent.setup()
      useProjectWorkflow.mockReturnValue({
        data: { project_id: 'test-project', has_workflow: true, state: failedInstance, workflows: ['feature'], all_workflows: { failinst: failedInstance } },
        isLoading: false,
      })

      renderPage()
      await user.click(screen.getByRole('button', { name: /Failed/ }))

      const row = screen.getByText('#failinst').closest('tr')!
      await user.click(within(row).getByRole('button'))
      await user.click(await screen.findByRole('button', { name: /^Delete$/ }))

      expect(deleteMutate).toHaveBeenCalledOnce()
      expect(deleteMutate).toHaveBeenCalledWith({ projectId: 'test-project', instanceId: 'failinst' })
    })

    it('does NOT call deleteMutation.mutate when dialog is cancelled', async () => {
      const user = userEvent.setup()
      useProjectWorkflow.mockReturnValue({
        data: { project_id: 'test-project', has_workflow: true, state: failedInstance, workflows: ['feature'], all_workflows: { failinst: failedInstance } },
        isLoading: false,
      })

      renderPage()
      await user.click(screen.getByRole('button', { name: /Failed/ }))

      const row = screen.getByText('#failinst').closest('tr')!
      await user.click(within(row).getByRole('button'))
      await user.click(await screen.findByRole('button', { name: /Cancel/ }))

      expect(deleteMutate).not.toHaveBeenCalled()
    })

    it('calls deleteMutation.mutate with completed instance id when confirmed on Completed tab', async () => {
      const user = userEvent.setup()
      useProjectWorkflow.mockReturnValue({
        data: { project_id: 'test-project', has_workflow: true, state: completedInstance, workflows: ['bugfix'], all_workflows: { compinst: completedInstance } },
        isLoading: false,
      })

      renderPage()
      await user.click(screen.getByRole('button', { name: /Completed/ }))

      const row = screen.getByText('#compinst').closest('tr')!
      await user.click(within(row).getByRole('button'))
      await user.click(await screen.findByRole('button', { name: /^Delete$/ }))

      expect(deleteMutate).toHaveBeenCalledWith({ projectId: 'test-project', instanceId: 'compinst' })
    })
  })
})
