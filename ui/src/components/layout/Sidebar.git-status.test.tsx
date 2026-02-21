import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Sidebar } from './Sidebar'
import type { StatusResponse } from '@/types/ticket'

// Mock useStatus and useProjectWorkflow hooks
const mockUseStatus = vi.fn()
const mockUseProjectWorkflow = vi.fn()
vi.mock('@/hooks/useTickets', () => ({
  useStatus: () => mockUseStatus(),
  useProjectWorkflow: () => mockUseProjectWorkflow(),
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

function createMockStatus(overrides: Partial<StatusResponse> = {}): StatusResponse {
  return {
    counts: {
      open: 5,
      in_progress: 0,
      closed: 10,
      blocked: 2,
      total: 17,
    },
    ready_count: 5,
    pending_tickets: [],
    recent_closed: [],
    ...overrides,
  }
}

function renderSidebar(initialRoute = '/') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialRoute]}>
        <Sidebar />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('Sidebar - Git Status Navigation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseStatus.mockReturnValue({ data: createMockStatus() })
    mockUseProjectWorkflow.mockReturnValue({ data: undefined })
    // Reset to project with default_branch
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

  it('shows Git Status nav item when project has default_branch', () => {
    renderSidebar()

    const gitStatusLink = screen.getByRole('link', { name: /git status/i })
    expect(gitStatusLink).toBeInTheDocument()
    expect(gitStatusLink).toHaveAttribute('href', '/git-status')
  })

  it('hides Git Status nav item when project has no default_branch', () => {
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

    renderSidebar()

    const gitStatusLink = screen.queryByRole('link', { name: /git status/i })
    expect(gitStatusLink).not.toBeInTheDocument()
  })

  it('hides Git Status nav item when no project is selected', () => {
    mockCurrentProject = ''

    renderSidebar()

    const gitStatusLink = screen.queryByRole('link', { name: /git status/i })
    expect(gitStatusLink).not.toBeInTheDocument()
  })

  it('highlights Git Status nav item when on /git-status route', () => {
    renderSidebar('/git-status')

    const gitStatusLink = screen.getByRole('link', { name: /git status/i })
    expect(gitStatusLink).toHaveClass('bg-muted', 'text-foreground')
  })

  it('shows Git Status nav item after Project Workflows', () => {
    renderSidebar()

    const allLinks = screen.getAllByRole('link')
    const projectWorkflowsIndex = allLinks.findIndex((link) =>
      link.textContent?.includes('Project Workflows')
    )
    const gitStatusIndex = allLinks.findIndex((link) =>
      link.textContent?.includes('Git Status')
    )

    expect(projectWorkflowsIndex).toBeGreaterThan(-1)
    expect(gitStatusIndex).toBeGreaterThan(-1)
    expect(gitStatusIndex).toBeGreaterThan(projectWorkflowsIndex)
  })
})
