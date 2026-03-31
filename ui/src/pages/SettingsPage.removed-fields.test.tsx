import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SettingsPage } from './SettingsPage'
import * as projectsApi from '@/api/projects'
import type { Project } from '@/api/projects'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector?: (s: { currentProject: string; setCurrentProject: (id: string) => void; loadProjects: () => void }) => unknown) => {
    const store = {
      currentProject: 'test-project',
      setCurrentProject: vi.fn(),
      loadProjects: vi.fn(),
    }
    return selector ? selector(store) : store
  },
}))

vi.mock('@/api/projects', async () => {
  const actual = await vi.importActual('@/api/projects')
  return {
    ...actual,
    listProjects: vi.fn(),
    createProject: vi.fn(),
    updateProject: vi.fn(),
    deleteProject: vi.fn(),
  }
})

vi.mock('@/api/systemAgentDefs', () => ({
  listSystemAgentDefs: vi.fn().mockResolvedValue([]),
  createSystemAgentDef: vi.fn(),
  updateSystemAgentDef: vi.fn(),
  deleteSystemAgentDef: vi.fn(),
}))

function makeProject(overrides: Partial<Project> = {}): Project {
  return {
    id: 'test-project',
    name: 'Test Project',
    root_path: '/test/path',
    default_branch: 'main',
    use_git_worktrees: false,
    use_docker_isolation: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SettingsPage — removed fields (default_workflow, use_docker_isolation)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('Create form', () => {
    it('does not render Default Workflow input', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      await userEvent.click(screen.getByRole('button', { name: /new project/i }))

      expect(screen.queryByText(/default workflow/i)).not.toBeInTheDocument()
    })

    it('does not render Docker Isolation toggle', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      await userEvent.click(screen.getByRole('button', { name: /new project/i }))

      expect(screen.queryByRole('switch', { name: /docker/i })).not.toBeInTheDocument()
    })
  })

  describe('Edit form', () => {
    it('does not render Default Workflow input', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [makeProject()] })
      renderPage()

      await screen.findByText('Test Project')
      await user.click(screen.getByRole('button', { name: '' }))

      expect(screen.queryByText(/default workflow/i)).not.toBeInTheDocument()
    })

    it('does not render Docker Isolation toggle', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [makeProject()] })
      renderPage()

      await screen.findByText('Test Project')
      await user.click(screen.getByRole('button', { name: '' }))

      expect(screen.queryByRole('switch', { name: /docker/i })).not.toBeInTheDocument()
    })
  })

  describe('Display mode', () => {
    it('does not show Docker metadata even when use_docker_isolation is true', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject({ use_docker_isolation: true })],
      })
      renderPage()

      await screen.findByText('Test Project')
      expect(screen.queryByText(/docker/i)).not.toBeInTheDocument()
    })

    it('does not show Default Workflow metadata in project display', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject()],
      })
      renderPage()

      await screen.findByText('Test Project')
      expect(screen.queryByText(/workflow/i)).not.toBeInTheDocument()
    })
  })
})
