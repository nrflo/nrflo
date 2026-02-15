import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ProjectWorkflowsPage } from './ProjectWorkflowsPage'
import type { ProjectWorkflowResponse, WorkflowState } from '@/types/workflow'
import type { Project } from '@/api/projects'

// Mock dependencies
let mockProjects: Project[] = []
let mockCurrentProject = 'test-project'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: {
    currentProject: string
    projects: Project[]
    projectsLoaded: boolean
  }) => unknown) =>
    selector({
      currentProject: mockCurrentProject,
      projects: mockProjects,
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
    useRestartProjectAgent: vi.fn(),
    useRetryFailedProjectAgent: vi.fn(),
  }
})

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn().mockResolvedValue({
    feature: {
      description: 'Feature workflow',
      scope_type: 'project',
      phases: [{ id: 'setup', agent: 'setup', layer: 0 }],
    },
  }),
}))

vi.mock('./WorkflowTabContent', () => ({
  WorkflowTabContent: () => <div data-testid="workflow-tab-content">Workflow Tab Content</div>,
}))

vi.mock('./GitStatusTabContent', () => ({
  GitStatusTabContent: ({ projectId }: { projectId: string }) => (
    <div data-testid="git-status-tab-content" data-project-id={projectId}>
      Git Status Tab Content
    </div>
  ),
}))

const sampleWorkflowState: WorkflowState = {
  workflow: 'feature',
  instance_id: 'instance-1',
  version: 4,
  scope_type: 'project',
  current_phase: 'implementation',
  status: 'active',
  phases: {
    investigation: { status: 'completed', result: 'pass' },
    implementation: { status: 'in_progress' },
  },
  phase_order: ['investigation', 'implementation', 'verification'],
  active_agents: {},
  findings: {},
}

const emptyWorkflowResponse: ProjectWorkflowResponse = {
  project_id: 'test-project',
  has_workflow: false,
  state: null,
  workflows: [],
  all_workflows: {},
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectWorkflowsPage />
    </QueryClientProvider>
  )
}

describe('ProjectWorkflowsPage - Git Status Tab', () => {
  let useProjectWorkflow: any
  let useProjectAgentSessions: any
  let useRunProjectWorkflow: any
  let useStopProjectWorkflow: any
  let useRetryFailedProjectAgent: any

  beforeEach(async () => {
    const hooks = await import('@/hooks/useTickets')
    useProjectWorkflow = hooks.useProjectWorkflow as any
    useProjectAgentSessions = hooks.useProjectAgentSessions as any
    useRunProjectWorkflow = hooks.useRunProjectWorkflow as any
    useStopProjectWorkflow = hooks.useStopProjectWorkflow as any
    useRetryFailedProjectAgent = hooks.useRetryFailedProjectAgent as any

    vi.clearAllMocks()

    // Reset to default: project WITH default_branch
    mockCurrentProject = 'test-project'
    mockProjects = [
      {
        id: 'test-project',
        name: 'Test Project',
        root_path: '/test',
        default_workflow: 'feature',
        default_branch: 'main',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    ]

    useProjectWorkflow.mockReturnValue({
      data: emptyWorkflowResponse,
      isLoading: false,
    })

    useProjectAgentSessions.mockReturnValue({
      data: { project_id: 'test-project', sessions: [] },
      isLoading: false,
    })

    useRunProjectWorkflow.mockReturnValue({
      mutateAsync: vi.fn().mockResolvedValue({ instance_id: 'new-instance', status: 'started' }),
      isPending: false,
      isError: false,
      error: null,
    })

    useStopProjectWorkflow.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    })

    useRetryFailedProjectAgent.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    })
  })

  describe('Git Status Tab Visibility', () => {
    it('renders Git Status tab when project has default_branch', async () => {
      renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })
    })

    it('does not render Git Status tab when project has no default_branch', async () => {
      mockProjects = [
        {
          id: 'test-project',
          name: 'Test Project',
          root_path: '/test',
          default_workflow: 'feature',
          default_branch: null,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ]

      renderPage()

      await waitFor(() => {
        expect(screen.queryByRole('button', { name: /Git Status/ })).not.toBeInTheDocument()
      })
    })

    it('does not render Git Status tab when default_branch is empty string', async () => {
      mockProjects = [
        {
          id: 'test-project',
          name: 'Test Project',
          root_path: '/test',
          default_workflow: 'feature',
          default_branch: '',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ]

      renderPage()

      await waitFor(() => {
        expect(screen.queryByRole('button', { name: /Git Status/ })).not.toBeInTheDocument()
      })
    })

    it('hides Git Status tab when project has no default_branch set', () => {
      mockProjects = [
        {
          id: 'no-branch-project',
          name: 'No Branch Project',
          root_path: '/test',
          default_workflow: null,
          default_branch: null,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ]
      mockCurrentProject = 'no-branch-project'

      renderPage()

      expect(screen.queryByRole('button', { name: /Git Status/ })).not.toBeInTheDocument()
    })

    it('shows Git Status tab when project list is empty but projectsLoaded is true', () => {
      mockProjects = []

      renderPage()

      // Tab should not appear when no projects exist
      expect(screen.queryByRole('button', { name: /Git Status/ })).not.toBeInTheDocument()
    })
  })

  describe('Git Status Tab Button Styling', () => {
    it('follows same styling pattern as other tab buttons', async () => {
      renderPage()

      await waitFor(() => {
        const gitTab = screen.getByRole('button', { name: /Git Status/ })
        expect(gitTab).toHaveClass('flex', 'items-center', 'gap-2', 'px-4', 'py-2')
        expect(gitTab).toHaveClass('text-sm', 'font-medium', 'border-b-2', 'transition-colors')
      })
    })

    it('shows GitCommitHorizontal icon', async () => {
      const { container } = renderPage()

      await waitFor(() => {
        const gitTab = screen.getByRole('button', { name: /Git Status/ })
        const icon = gitTab.querySelector('.lucide-git-commit-horizontal')
        expect(icon).toBeInTheDocument()
      })
    })

    it('has inactive styling when not selected', async () => {
      renderPage()

      await waitFor(() => {
        const gitTab = screen.getByRole('button', { name: /Git Status/ })
        expect(gitTab).toHaveClass('border-transparent', 'text-muted-foreground')
      })
    })

    it('has active styling when selected', async () => {
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      const gitTab = screen.getByRole('button', { name: /Git Status/ })
      await user.click(gitTab)

      expect(gitTab).toHaveClass('border-primary', 'text-primary')
    })
  })

  describe('Git Status Tab Switching', () => {
    it('switches to git tab when clicked', async () => {
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      const gitTab = screen.getByRole('button', { name: /Git Status/ })
      await user.click(gitTab)

      expect(screen.getByTestId('git-status-tab-content')).toBeInTheDocument()
      expect(screen.queryByTestId('workflow-tab-content')).not.toBeInTheDocument()
    })

    it('passes correct projectId to GitStatusTabContent', async () => {
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      await user.click(screen.getByRole('button', { name: /Git Status/ }))

      const gitStatusContent = screen.getByTestId('git-status-tab-content')
      expect(gitStatusContent).toHaveAttribute('data-project-id', 'test-project')
    })

    it('hides GitStatusTabContent when switching to Run tab', async () => {
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      // Switch to git tab
      await user.click(screen.getByRole('button', { name: /Git Status/ }))
      expect(screen.getByTestId('git-status-tab-content')).toBeInTheDocument()

      // Switch back to Run tab
      await user.click(screen.getByRole('button', { name: /Run Workflow/ }))
      expect(screen.queryByTestId('git-status-tab-content')).not.toBeInTheDocument()
    })

    it('hides GitStatusTabContent when switching to Running tab', async () => {
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      // Switch to git tab
      await user.click(screen.getByRole('button', { name: /Git Status/ }))
      expect(screen.getByTestId('git-status-tab-content')).toBeInTheDocument()

      // Switch to Running tab
      await user.click(screen.getByRole('button', { name: /Running/ }))
      expect(screen.queryByTestId('git-status-tab-content')).not.toBeInTheDocument()
    })

    it('hides GitStatusTabContent when switching to Completed tab', async () => {
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      // Switch to git tab
      await user.click(screen.getByRole('button', { name: /Git Status/ }))
      expect(screen.getByTestId('git-status-tab-content')).toBeInTheDocument()

      // Switch to Completed tab
      await user.click(screen.getByRole('button', { name: /Completed/ }))
      expect(screen.queryByTestId('git-status-tab-content')).not.toBeInTheDocument()
    })
  })

  describe('Edge Case: Project Switch While on Git Tab', () => {
    it('resets to Run tab when switching to project without default_branch while on git tab', async () => {
      const user = userEvent.setup()

      // Start with project that has default_branch
      mockProjects = [
        {
          id: 'project-with-branch',
          name: 'Project With Branch',
          root_path: '/test',
          default_workflow: 'feature',
          default_branch: 'main',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ]
      mockCurrentProject = 'project-with-branch'

      const { rerender } = renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      // Switch to git tab
      await user.click(screen.getByRole('button', { name: /Git Status/ }))
      expect(screen.getByTestId('git-status-tab-content')).toBeInTheDocument()

      // Simulate project change to one without default_branch
      mockProjects = [
        {
          id: 'project-without-branch',
          name: 'Project Without Branch',
          root_path: '/test2',
          default_workflow: 'feature',
          default_branch: null,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ]
      mockCurrentProject = 'project-without-branch'

      // Force re-render
      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      rerender(
        <QueryClientProvider client={queryClient}>
          <ProjectWorkflowsPage />
        </QueryClientProvider>
      )

      // Should reset to Run tab (git tab should disappear, Run tab should be active)
      await waitFor(() => {
        expect(screen.queryByRole('button', { name: /Git Status/ })).not.toBeInTheDocument()
      })

      // Run tab should be active
      const runTab = screen.getByRole('button', { name: /Run Workflow/ })
      expect(runTab).toHaveClass('border-primary', 'text-primary')

      // Git status content should not be rendered
      expect(screen.queryByTestId('git-status-tab-content')).not.toBeInTheDocument()
    })

    it('keeps git tab visible when switching to another project with default_branch', async () => {
      const user = userEvent.setup()

      // Start with first project
      mockProjects = [
        {
          id: 'project-1',
          name: 'Project 1',
          root_path: '/test1',
          default_workflow: 'feature',
          default_branch: 'main',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ]
      mockCurrentProject = 'project-1'

      const { rerender } = renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      // Switch to git tab
      await user.click(screen.getByRole('button', { name: /Git Status/ }))
      expect(screen.getByTestId('git-status-tab-content')).toBeInTheDocument()
      expect(screen.getByTestId('git-status-tab-content')).toHaveAttribute('data-project-id', 'project-1')

      // Switch to another project that ALSO has default_branch
      mockProjects = [
        {
          id: 'project-2',
          name: 'Project 2',
          root_path: '/test2',
          default_workflow: 'feature',
          default_branch: 'develop',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ]
      mockCurrentProject = 'project-2'

      // Force re-render
      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      rerender(
        <QueryClientProvider client={queryClient}>
          <ProjectWorkflowsPage />
        </QueryClientProvider>
      )

      // Git tab should still be visible and content should update to new projectId
      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      // Content should show new project
      const gitStatusContent = screen.getByTestId('git-status-tab-content')
      expect(gitStatusContent).toHaveAttribute('data-project-id', 'project-2')
    })
  })

  describe('Container Layout for Git Tab', () => {
    it('uses narrow layout (max-w-7xl) when git tab is active', async () => {
      const user = userEvent.setup()
      const { container } = renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      // Switch to git tab
      await user.click(screen.getByRole('button', { name: /Git Status/ }))

      // Check container has narrow layout
      const pageContainer = container.firstChild as HTMLElement
      expect(pageContainer).toHaveClass('max-w-7xl', 'mx-auto', 'p-6', 'space-y-6')
      expect(pageContainer).not.toHaveClass('max-w-full')
    })

    it('uses narrow layout on Run tab', async () => {
      const { container } = renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Run Workflow/ })).toBeInTheDocument()
      })

      const pageContainer = container.firstChild as HTMLElement
      expect(pageContainer).toHaveClass('max-w-7xl', 'mx-auto', 'p-6', 'space-y-6')
    })

    it('uses wide layout on Running tab with active phase', async () => {
      const user = userEvent.setup()

      // Set up workflow with active phase
      useProjectWorkflow.mockReturnValue({
        data: {
          project_id: 'test-project',
          has_workflow: true,
          state: sampleWorkflowState,
          workflows: ['feature'],
          all_workflows: { 'instance-1': sampleWorkflowState },
        },
        isLoading: false,
      })

      const { container } = renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Running/ })).toBeInTheDocument()
      })

      // Switch to Running tab
      await user.click(screen.getByRole('button', { name: /Running/ }))

      const pageContainer = container.firstChild as HTMLElement
      expect(pageContainer).toHaveClass('max-w-full', 'px-4', 'space-y-6')
      expect(pageContainer).not.toHaveClass('max-w-7xl')
    })

    it('does not use wide layout on git tab even with active phase in workflow', async () => {
      const user = userEvent.setup()

      // Set up workflow with active phase
      useProjectWorkflow.mockReturnValue({
        data: {
          project_id: 'test-project',
          has_workflow: true,
          state: sampleWorkflowState,
          workflows: ['feature'],
          all_workflows: { 'instance-1': sampleWorkflowState },
        },
        isLoading: false,
      })

      const { container } = renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      // Switch to git tab
      await user.click(screen.getByRole('button', { name: /Git Status/ }))

      // Should still use narrow layout (git tab doesn't care about active phase)
      const pageContainer = container.firstChild as HTMLElement
      expect(pageContainer).toHaveClass('max-w-7xl', 'mx-auto', 'p-6', 'space-y-6')
      expect(pageContainer).not.toHaveClass('max-w-full')
    })
  })

  describe('Full User Flow - Git Tab Integration', () => {
    it('navigates through all tabs including git tab', async () => {
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      // Start on Run tab
      expect(screen.getByRole('button', { name: /Run Workflow/ })).toHaveClass('border-primary')

      // Navigate to Running tab
      await user.click(screen.getByRole('button', { name: /Running/ }))
      expect(screen.getByRole('button', { name: /Running/ })).toHaveClass('border-primary')

      // Navigate to Completed tab
      await user.click(screen.getByRole('button', { name: /Completed/ }))
      expect(screen.getByRole('button', { name: /Completed/ })).toHaveClass('border-primary')

      // Navigate to Git tab
      await user.click(screen.getByRole('button', { name: /Git Status/ }))
      expect(screen.getByRole('button', { name: /Git Status/ })).toHaveClass('border-primary')
      expect(screen.getByTestId('git-status-tab-content')).toBeInTheDocument()

      // Navigate back to Run tab
      await user.click(screen.getByRole('button', { name: /Run Workflow/ }))
      expect(screen.getByRole('button', { name: /Run Workflow/ })).toHaveClass('border-primary')
      expect(screen.queryByTestId('git-status-tab-content')).not.toBeInTheDocument()
    })

    it('git tab appears in correct position (after Completed tab)', async () => {
      const { container } = renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      const tabContainer = container.querySelector('.border-b.border-border > .flex.gap-1')
      const buttons = tabContainer?.querySelectorAll('button')
      const buttonTexts = Array.from(buttons || []).map((btn) => btn.textContent)

      expect(buttonTexts).toEqual([
        'Run Workflow',
        'Running (0)',
        'Completed (0)',
        'Git Status',
      ])
    })
  })

  describe('Multiple Projects with Different Branch Configs', () => {
    it('shows/hides git tab correctly based on different project branch configs', async () => {
      // Project with branch
      mockProjects = [
        {
          id: 'with-branch',
          name: 'With Branch',
          root_path: '/test1',
          default_workflow: 'feature',
          default_branch: 'main',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
        {
          id: 'without-branch',
          name: 'Without Branch',
          root_path: '/test2',
          default_workflow: 'feature',
          default_branch: null,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ]

      // Test with first project (has branch)
      mockCurrentProject = 'with-branch'
      const { rerender } = renderPage()

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Git Status/ })).toBeInTheDocument()
      })

      // Switch to second project (no branch)
      mockCurrentProject = 'without-branch'

      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      rerender(
        <QueryClientProvider client={queryClient}>
          <ProjectWorkflowsPage />
        </QueryClientProvider>
      )

      await waitFor(() => {
        expect(screen.queryByRole('button', { name: /Git Status/ })).not.toBeInTheDocument()
      })
    })
  })
})
