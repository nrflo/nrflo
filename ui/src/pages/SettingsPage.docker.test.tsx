import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SettingsPage } from './SettingsPage'
import * as projectsApi from '@/api/projects'
import type { Project } from '@/api/projects'

const mockSetCurrentProject = vi.fn()
const mockLoadProjects = vi.fn()

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector?: (s: { currentProject: string; setCurrentProject: (id: string) => void; loadProjects: () => void }) => unknown) => {
    const store = {
      currentProject: 'test-project',
      setCurrentProject: mockSetCurrentProject,
      loadProjects: mockLoadProjects,
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
    default_workflow: 'feature',
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
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SettingsPage - use_docker_isolation toggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('Create form', () => {
    it('toggle defaults to off', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      await userEvent.click(screen.getByRole('button', { name: /new project/i }))

      const toggle = screen.getByRole('switch', { name: /use docker isolation/i })
      expect(toggle).toHaveAttribute('aria-checked', 'false')
    })

    it('toggle is always enabled (no dependency on default_branch)', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      await userEvent.click(screen.getByRole('button', { name: /new project/i }))

      // default_branch is empty — docker toggle should still be enabled
      const toggle = screen.getByRole('switch', { name: /use docker isolation/i })
      expect(toggle).not.toBeDisabled()
    })

    it('saving with toggle on sends use_docker_isolation: true', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      vi.mocked(projectsApi.createProject).mockResolvedValue(
        makeProject({ id: 'new-project', use_docker_isolation: true })
      )

      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      await user.click(screen.getByRole('button', { name: /new project/i }))

      await user.type(screen.getByPlaceholderText('project-id'), 'new-project')
      await user.type(screen.getByPlaceholderText('main'), 'main')

      const toggle = screen.getByRole('switch', { name: /use docker isolation/i })
      await user.click(toggle)
      expect(toggle).toHaveAttribute('aria-checked', 'true')

      await user.click(screen.getByRole('button', { name: /^create$/i }))

      await waitFor(() => {
        expect(projectsApi.createProject).toHaveBeenCalledWith(
          expect.objectContaining({ use_docker_isolation: true })
        )
      })
    })

    it('saving with toggle off sends use_docker_isolation: false', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      vi.mocked(projectsApi.createProject).mockResolvedValue(
        makeProject({ id: 'new-project', use_docker_isolation: false })
      )

      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      await user.click(screen.getByRole('button', { name: /new project/i }))

      await user.type(screen.getByPlaceholderText('project-id'), 'new-project')
      await user.type(screen.getByPlaceholderText('main'), 'main')

      // Leave toggle off
      const toggle = screen.getByRole('switch', { name: /use docker isolation/i })
      expect(toggle).toHaveAttribute('aria-checked', 'false')

      await user.click(screen.getByRole('button', { name: /^create$/i }))

      await waitFor(() => {
        expect(projectsApi.createProject).toHaveBeenCalledWith(
          expect.objectContaining({ use_docker_isolation: false })
        )
      })
    })
  })

  describe('Edit form', () => {
    it('toggle reflects current project value (true)', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject({ use_docker_isolation: true })],
      })

      renderPage()

      await screen.findByText('Test Project')
      await user.click(screen.getByRole('button', { name: '' }))

      const toggle = screen.getByRole('switch', { name: /use docker isolation/i })
      expect(toggle).toHaveAttribute('aria-checked', 'true')
    })

    it('toggle reflects current project value (false)', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject({ use_docker_isolation: false })],
      })

      renderPage()

      await screen.findByText('Test Project')
      await user.click(screen.getByRole('button', { name: '' }))

      const toggle = screen.getByRole('switch', { name: /use docker isolation/i })
      expect(toggle).toHaveAttribute('aria-checked', 'false')
    })

    it('update request includes use_docker_isolation: true when toggled on', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject({ use_docker_isolation: false })],
      })
      vi.mocked(projectsApi.updateProject).mockResolvedValue(
        makeProject({ use_docker_isolation: true })
      )

      renderPage()

      await screen.findByText('Test Project')
      await user.click(screen.getByRole('button', { name: '' }))

      const toggle = screen.getByRole('switch', { name: /use docker isolation/i })
      await user.click(toggle)
      expect(toggle).toHaveAttribute('aria-checked', 'true')

      await user.click(screen.getByRole('button', { name: /save/i }))

      await waitFor(() => {
        expect(projectsApi.updateProject).toHaveBeenCalledWith(
          'test-project',
          expect.objectContaining({ use_docker_isolation: true })
        )
      })
    })

    it('update request includes use_docker_isolation: false when toggled off', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject({ use_docker_isolation: true })],
      })
      vi.mocked(projectsApi.updateProject).mockResolvedValue(
        makeProject({ use_docker_isolation: false })
      )

      renderPage()

      await screen.findByText('Test Project')
      await user.click(screen.getByRole('button', { name: '' }))

      const toggle = screen.getByRole('switch', { name: /use docker isolation/i })
      await user.click(toggle)
      expect(toggle).toHaveAttribute('aria-checked', 'false')

      await user.click(screen.getByRole('button', { name: /save/i }))

      await waitFor(() => {
        expect(projectsApi.updateProject).toHaveBeenCalledWith(
          'test-project',
          expect.objectContaining({ use_docker_isolation: false })
        )
      })
    })

    it('toggle stays enabled even when default_branch is empty', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject({ default_branch: null, use_docker_isolation: false })],
      })

      renderPage()

      await screen.findByText('Test Project')
      await user.click(screen.getByRole('button', { name: '' }))

      const toggle = screen.getByRole('switch', { name: /use docker isolation/i })
      expect(toggle).not.toBeDisabled()
    })
  })

  describe('Display mode', () => {
    it('shows "Docker: enabled" when use_docker_isolation is true', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject({ use_docker_isolation: true })],
      })

      renderPage()

      await screen.findByText('Test Project')
      expect(screen.getByText('Docker: enabled')).toBeInTheDocument()
    })

    it('does not show docker info when use_docker_isolation is false', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject({ use_docker_isolation: false })],
      })

      renderPage()

      await screen.findByText('Test Project')
      expect(screen.queryByText(/docker/i)).not.toBeInTheDocument()
    })

    it('shows docker alongside other metadata', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [
          makeProject({
            root_path: '/custom/path',
            default_workflow: 'bugfix',
            default_branch: 'develop',
            use_git_worktrees: true,
            use_docker_isolation: true,
          }),
        ],
      })

      renderPage()

      await screen.findByText('Test Project')
      expect(screen.getByText('Path: /custom/path')).toBeInTheDocument()
      expect(screen.getByText('Workflow: bugfix')).toBeInTheDocument()
      expect(screen.getByText('Branch: develop')).toBeInTheDocument()
      expect(screen.getByText('Worktrees: enabled')).toBeInTheDocument()
      expect(screen.getByText('Docker: enabled')).toBeInTheDocument()
    })
  })
})
