import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ProjectWorkflowsPage } from './ProjectWorkflowsPage'
import type { InputArtifactRef } from '@/types/artifact'

// ── Mocks ─────────────────────────────────────────────────────────────────────

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
    'proj-wf': { description: 'Project workflow', scope_type: 'project', phases: [{ id: 'setup', agent: 'setup', layer: 0 }] },
  }),
}))

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn().mockResolvedValue([
    { id: 'setup', model: 'sonnet', timeout: 300, prompt: '', workflow_id: 'proj-wf', project_id: 'test-project', created_at: '', updated_at: '' },
  ]),
}))

const mockCancelUpload = vi.fn()
vi.mock('@/api/artifacts', () => ({
  cancelUpload: (...args: unknown[]) => mockCancelUpload(...args),
}))

vi.mock('./WorkflowTabContent', () => ({
  WorkflowTabContent: () => <div data-testid="workflow-tab-content" />,
}))

// Controllable ArtifactUploader stub
let uploaderOnChange: ((refs: InputArtifactRef[], hasPending: boolean) => void) | null = null
vi.mock('@/components/workflow/ArtifactUploader', () => ({
  ArtifactUploader: ({ onChange }: { onChange: (refs: InputArtifactRef[], hasPending: boolean) => void }) => {
    uploaderOnChange = onChange
    return (
      <div data-testid="artifact-uploader">
        <button type="button" onClick={() => onChange([], true)}>sim-pending</button>
        <button type="button" onClick={() => onChange([{ upload_id: 'uid-1', name: 'f.txt' }], false)}>sim-done</button>
      </div>
    )
  },
}))

// ── Helpers ───────────────────────────────────────────────────────────────────

const emptyWorkflowData = {
  project_id: 'test-project',
  has_workflow: false,
  state: null,
  workflows: [],
  all_workflows: {},
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectWorkflowsPage />
    </QueryClientProvider>
  )
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('ProjectWorkflowsPage — artifact upload integration', () => {
  let mockMutateAsync: ReturnType<typeof vi.fn>

  beforeEach(async () => {
    vi.clearAllMocks()
    uploaderOnChange = null
    mockCancelUpload.mockResolvedValue(undefined)

    mockMutateAsync = vi.fn().mockResolvedValue({ instance_id: 'new-inst', session_id: null })

    const hooks = await import('@/hooks/useTickets')
    ;(hooks.useProjectWorkflow as any).mockReturnValue({ data: emptyWorkflowData, isLoading: false })
    ;(hooks.useProjectAgentSessions as any).mockReturnValue({ data: { project_id: 'test-project', sessions: [] }, isLoading: false })
    ;(hooks.useRunProjectWorkflow as any).mockReturnValue({ mutateAsync: mockMutateAsync, isPending: false, isError: false, error: null })
    ;(hooks.useStopProjectWorkflow as any).mockReturnValue({ mutate: vi.fn(), isPending: false })
    ;(hooks.useRetryFailedProjectAgent as any).mockReturnValue({ mutate: vi.fn(), isPending: false, variables: null })
    ;(hooks.useTakeControlProject as any).mockReturnValue({ mutate: vi.fn(), isPending: false })
    ;(hooks.useExitInteractiveProject as any).mockReturnValue({ mutate: vi.fn(), isPending: false })
    ;(hooks.useResumeSessionProject as any).mockReturnValue({ mutate: vi.fn(), isPending: false })
    ;(hooks.useDeleteProjectWorkflowInstance as any).mockReturnValue({ mutate: vi.fn(), isPending: false })
    ;(hooks.useSetStopEndlessLoopAfterIteration as any).mockReturnValue({ mutate: vi.fn(), isPending: false })
    ;(hooks.useProjectFindings as any).mockReturnValue({ data: {}, isLoading: false })
  })

  it('Run button is disabled while upload is pending', async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => expect(screen.getByRole('button', { name: /^Run$/ })).not.toBeDisabled())

    await user.click(screen.getByRole('button', { name: 'sim-pending' }))

    expect(screen.getByRole('button', { name: /^Run$/ })).toBeDisabled()
  })

  it('Run button is re-enabled after upload completes', async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => expect(screen.getByRole('button', { name: /^Run$/ })).not.toBeDisabled())

    await user.click(screen.getByRole('button', { name: 'sim-pending' }))
    expect(screen.getByRole('button', { name: /^Run$/ })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'sim-done' }))
    expect(screen.getByRole('button', { name: /^Run$/ })).not.toBeDisabled()
  })

  it('includes input_artifacts in mutateAsync when artifacts are staged', async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => expect(screen.getByRole('button', { name: /^Run$/ })).not.toBeDisabled())

    await user.click(screen.getByRole('button', { name: 'sim-done' }))
    await user.click(screen.getByRole('button', { name: /^Run$/ }))

    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalled())
    const callArgs = mockMutateAsync.mock.calls[0][0]
    expect(callArgs.params.input_artifacts).toEqual([{ upload_id: 'uid-1', name: 'f.txt' }])
  })

  it('does not include input_artifacts when no artifacts staged', async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => expect(screen.getByRole('button', { name: /^Run$/ })).not.toBeDisabled())

    await user.click(screen.getByRole('button', { name: /^Run$/ }))

    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalled())
    const callArgs = mockMutateAsync.mock.calls[0][0]
    expect(callArgs.params.input_artifacts).toBeUndefined()
  })

  it('calls cancelUpload for staged artifact when leaving Run tab without submitting', async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => expect(screen.getByRole('button', { name: /^Run$/ })).not.toBeDisabled())

    await user.click(screen.getByRole('button', { name: 'sim-done' }))

    // Switch away from Run tab without clicking Run
    await user.click(screen.getByRole('button', { name: /Running/ }))

    await waitFor(() => {
      expect(mockCancelUpload).toHaveBeenCalledWith('uid-1')
    })
  })

  it('does NOT call cancelUpload after a successful run', async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => expect(screen.getByRole('button', { name: /^Run$/ })).not.toBeDisabled())

    await user.click(screen.getByRole('button', { name: 'sim-done' }))
    await user.click(screen.getByRole('button', { name: /^Run$/ }))

    // Wait for navigation to Running tab (happens on successful run)
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Running/ })).toHaveClass('border-primary')
    })

    expect(mockCancelUpload).not.toHaveBeenCalled()
  })

  it('successful run clears staged artifacts so next run has no input_artifacts', async () => {
    const user = userEvent.setup()
    const hooks = await import('@/hooks/useTickets')

    // After the first run succeeds, page switches to Running; we set up data for that
    ;(hooks.useProjectWorkflow as any).mockReturnValue({
      data: {
        project_id: 'test-project',
        has_workflow: true,
        state: { workflow: 'proj-wf', instance_id: 'new-inst', version: 4, scope_type: 'project', current_phase: 'setup', status: 'active', phases: {}, phase_order: [], active_agents: {}, findings: {} },
        workflows: ['proj-wf'],
        all_workflows: { 'new-inst': { workflow: 'proj-wf', instance_id: 'new-inst', version: 4, scope_type: 'project', current_phase: 'setup', status: 'active', phases: {}, phase_order: [], active_agents: {}, findings: {} } },
      },
      isLoading: false,
    })

    renderPage()

    await waitFor(() => expect(screen.getByRole('button', { name: /^Run$/ })).not.toBeDisabled())

    // Stage an artifact and run
    await user.click(screen.getByRole('button', { name: 'sim-done' }))
    await user.click(screen.getByRole('button', { name: /^Run$/ }))

    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalledTimes(1))

    // Navigate back to Run tab
    await user.click(screen.getByRole('button', { name: /Run Workflow/ }))

    await waitFor(() => expect(screen.getByRole('button', { name: /^Run$/ })).not.toBeDisabled())

    // Second run — no artifacts staged, so input_artifacts should be absent
    await user.click(screen.getByRole('button', { name: /^Run$/ }))

    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalledTimes(2))
    const secondCall = mockMutateAsync.mock.calls[1][0]
    expect(secondCall.params.input_artifacts).toBeUndefined()
  })
})
