import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { GitStatusPage } from './GitStatusPage'

// Mock GitStatusTabContent as a simple component
vi.mock('./GitStatusTabContent', () => ({
  GitStatusTabContent: ({ projectId }: { projectId: string }) => (
    <div data-testid="git-status-tab-content">
      GitStatusTabContent for project: {projectId}
    </div>
  ),
}))

// Mock store with selector pattern
let mockCurrentProject = 'test-project'
let mockProjects = [
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

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projects: unknown[] }) => unknown) =>
    selector({
      currentProject: mockCurrentProject,
      projects: mockProjects,
    }),
}))

describe('GitStatusPage', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    })
    // Reset to default state
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
  })

  function renderPage() {
    return render(
      <QueryClientProvider client={queryClient}>
        <GitStatusPage />
      </QueryClientProvider>
    )
  }

  it('renders GitStatusTabContent when project has default_branch', () => {
    renderPage()

    expect(screen.getByText('Git Status')).toBeInTheDocument()
    expect(screen.getByTestId('git-status-tab-content')).toBeInTheDocument()
    expect(screen.getByText(/GitStatusTabContent for project: test-project/)).toBeInTheDocument()
  })

  it('shows "No project selected" message when currentProject is empty', () => {
    mockCurrentProject = ''

    renderPage()

    expect(screen.getByText('Git Status')).toBeInTheDocument()
    expect(screen.getByText('No project selected')).toBeInTheDocument()
    expect(screen.queryByTestId('git-status-tab-content')).not.toBeInTheDocument()
  })

  it('shows "No default branch configured" message when project lacks default_branch', () => {
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

    expect(screen.getByText('Git Status')).toBeInTheDocument()
    expect(screen.getByText('No default branch configured for this project')).toBeInTheDocument()
    expect(screen.queryByTestId('git-status-tab-content')).not.toBeInTheDocument()
  })
})
