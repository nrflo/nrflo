import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
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
    useTakeControlProject: vi.fn(),
    useExitInteractiveProject: vi.fn(),
    useResumeSessionProject: vi.fn(),
    useDeleteProjectWorkflowInstance: vi.fn(),
    useSetStopEndlessLoopAfterIteration: vi.fn(),
    useProjectFindings: vi.fn(),
  }
})

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn().mockResolvedValue({
    feature: { description: 'Feature', scope_type: 'project', phases: [{ id: 'setup', agent: 'setup', layer: 0 }] },
  }),
}))

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn().mockResolvedValue([
    { id: 'setup', model: 'sonnet', timeout: 300, prompt: 'test', workflow_id: 'feature', project_id: 'test-project', created_at: '', updated_at: '' },
  ]),
}))

vi.mock('./WorkflowTabContent', () => ({
  WorkflowTabContent: () => <div data-testid="workflow-tab-content" />,
}))

type WorkflowStateOverrides = Partial<WorkflowState>

const makeInstance = (id: string, overrides: WorkflowStateOverrides = {}): WorkflowState => ({
  workflow: 'feature',
  instance_id: id,
  version: 4,
  scope_type: 'project',
  current_phase: 'setup',
  status: 'active',
  phases: { setup: { status: 'in_progress' } },
  phase_order: ['setup'],
  active_agents: {},
  findings: {},
  ...overrides,
})

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectWorkflowsPage />
    </QueryClientProvider>
  )
}

describe('ProjectWorkflowsPage — endless loop running controls', () => {
  let useProjectWorkflow: any
  let useSetStopEndlessLoopAfterIteration: any
  const stopEndlessMutate = vi.fn()

  beforeEach(async () => {
    const hooks = await import('@/hooks/useTickets')
    useProjectWorkflow = hooks.useProjectWorkflow as any
    useSetStopEndlessLoopAfterIteration = hooks.useSetStopEndlessLoopAfterIteration as any
    ;(hooks.useProjectAgentSessions as any).mockReturnValue({ data: { project_id: 'test-project', sessions: [] }, isLoading: false })
    ;(hooks.useRunProjectWorkflow as any).mockReturnValue({ mutateAsync: vi.fn(), isPending: false, isError: false, error: null })
    ;(hooks.useStopProjectWorkflow as any).mockReturnValue({ mutate: vi.fn(), isPending: false })
    ;(hooks.useRetryFailedProjectAgent as any).mockReturnValue({ mutate: vi.fn(), isPending: false, variables: null })
    ;(hooks.useTakeControlProject as any).mockReturnValue({ mutate: vi.fn(), isPending: false })
    ;(hooks.useExitInteractiveProject as any).mockReturnValue({ mutate: vi.fn(), isPending: false })
    ;(hooks.useResumeSessionProject as any).mockReturnValue({ mutate: vi.fn(), isPending: false })
    ;(hooks.useDeleteProjectWorkflowInstance as any).mockReturnValue({ mutate: vi.fn(), isPending: false })
    ;(hooks.useProjectFindings as any).mockReturnValue({ data: {}, isLoading: false })

    vi.clearAllMocks()
    stopEndlessMutate.mockReset()
    useSetStopEndlessLoopAfterIteration.mockReturnValue({ mutate: stopEndlessMutate, isPending: false })
  })

  const mountWith = async (instance: WorkflowState) => {
    useProjectWorkflow.mockReturnValue({
      data: {
        project_id: 'test-project',
        has_workflow: true,
        state: instance,
        workflows: ['feature'],
        all_workflows: { [instance.instance_id]: instance },
      },
      isLoading: false,
    })
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /Running/ }))
    await waitFor(() => expect(screen.getByTestId('workflow-tab-content')).toBeInTheDocument())
    return user
  }

  it('renders Endless-loop badge and stop-after-iteration checkbox when endless_loop=true and status=active', async () => {
    await mountWith(makeInstance('inst-endl1', { endless_loop: true, status: 'active' }))

    expect(screen.getByText('Endless loop')).toBeInTheDocument()
    const checkbox = screen.getByLabelText(/stop endless loop after current iteration/i) as HTMLInputElement
    expect(checkbox).toBeInTheDocument()
    expect(checkbox.checked).toBe(false)
  })

  it('reflects stop_endless_loop_after_iteration=true on the checkbox', async () => {
    await mountWith(makeInstance('inst-endl2', {
      endless_loop: true,
      status: 'active',
      stop_endless_loop_after_iteration: true,
    }))

    const checkbox = screen.getByLabelText(/stop endless loop after current iteration/i) as HTMLInputElement
    expect(checkbox.checked).toBe(true)
  })

  it('does not render badge/checkbox when endless_loop is false/undefined', async () => {
    await mountWith(makeInstance('inst-plain', { status: 'active' }))

    expect(screen.queryByText('Endless loop')).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/stop endless loop after current iteration/i)).not.toBeInTheDocument()
  })

  it('does not render badge/checkbox for non-active instance (completed)', async () => {
    const completed = makeInstance('inst-done', {
      endless_loop: true,
      status: 'completed',
      completed_at: '2026-01-01T05:00:00Z',
      phases: { setup: { status: 'completed', result: 'pass' } },
    })

    useProjectWorkflow.mockReturnValue({
      data: {
        project_id: 'test-project',
        has_workflow: true,
        state: completed,
        workflows: ['feature'],
        all_workflows: { 'inst-done': completed },
      },
      isLoading: false,
    })

    const user = userEvent.setup()
    renderPage()
    // Completed instances are on the Completed tab, not Running. Verify neither tab shows the control.
    await user.click(screen.getByRole('button', { name: /Running/ }))
    expect(screen.queryByText('Endless loop')).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/stop endless loop after current iteration/i)).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Completed/ }))
    expect(screen.queryByText('Endless loop')).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/stop endless loop after current iteration/i)).not.toBeInTheDocument()
  })

  it('toggling the checkbox calls the mutation with projectId, instanceId and stop=true', async () => {
    const user = await mountWith(makeInstance('inst-endl3', { endless_loop: true, status: 'active' }))

    const checkbox = screen.getByLabelText(/stop endless loop after current iteration/i)
    await user.click(checkbox)

    expect(stopEndlessMutate).toHaveBeenCalledTimes(1)
    expect(stopEndlessMutate).toHaveBeenCalledWith({
      projectId: 'test-project',
      instanceId: 'inst-endl3',
      stop: true,
    })
  })

  it('unchecking the checkbox calls the mutation with stop=false', async () => {
    const user = await mountWith(makeInstance('inst-endl4', {
      endless_loop: true,
      status: 'active',
      stop_endless_loop_after_iteration: true,
    }))

    const checkbox = screen.getByLabelText(/stop endless loop after current iteration/i)
    await user.click(checkbox)

    expect(stopEndlessMutate).toHaveBeenCalledWith({
      projectId: 'test-project',
      instanceId: 'inst-endl4',
      stop: false,
    })
  })

  it('disables the checkbox while the mutation is pending', async () => {
    useSetStopEndlessLoopAfterIteration.mockReturnValue({ mutate: stopEndlessMutate, isPending: true })
    await mountWith(makeInstance('inst-endl5', { endless_loop: true, status: 'active' }))

    const checkbox = screen.getByLabelText(/stop endless loop after current iteration/i) as HTMLInputElement
    expect(checkbox).toBeDisabled()
  })
})
