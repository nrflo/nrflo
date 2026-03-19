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

describe('SettingsPage - use_git_worktrees toggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('Create form', () => {
    it('toggle defaults to off', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      const newButton = screen.getByRole('button', { name: /new project/i })
      await userEvent.click(newButton)

      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      expect(toggle).toHaveAttribute('aria-checked', 'false')
    })

    it('toggle is disabled when default_branch is empty', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      const newButton = screen.getByRole('button', { name: /new project/i })
      await userEvent.click(newButton)

      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      expect(toggle).toBeDisabled()
    })

    it('toggle is enabled when default_branch is typed', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      const newButton = screen.getByRole('button', { name: /new project/i })
      await user.click(newButton)

      const branchInput = screen.getByPlaceholderText('main')
      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })

      expect(toggle).toBeDisabled()

      await user.type(branchInput, 'master')

      expect(toggle).not.toBeDisabled()
    })

    it('clearing default_branch auto-unchecks the toggle', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      const newButton = screen.getByRole('button', { name: /new project/i })
      await user.click(newButton)

      const branchInput = screen.getByPlaceholderText('main')
      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })

      // Type branch name and enable toggle
      await user.type(branchInput, 'main')
      await user.click(toggle)
      expect(toggle).toHaveAttribute('aria-checked', 'true')

      // Clear branch field
      await user.clear(branchInput)

      // Toggle should be unchecked and disabled
      expect(toggle).toHaveAttribute('aria-checked', 'false')
      expect(toggle).toBeDisabled()
    })

    it('saving with toggle on sends use_git_worktrees: true', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      vi.mocked(projectsApi.createProject).mockResolvedValue(
        makeProject({ id: 'new-project', use_git_worktrees: true })
      )

      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      const newButton = screen.getByRole('button', { name: /new project/i })
      await user.click(newButton)

      // Fill required fields
      await user.type(screen.getByPlaceholderText('project-id'), 'new-project')
      await user.type(screen.getByPlaceholderText('main'), 'main')

      // Enable toggle
      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      await user.click(toggle)
      expect(toggle).toHaveAttribute('aria-checked', 'true')

      // Create project
      const createButton = screen.getByRole('button', { name: /^create$/i })
      await user.click(createButton)

      await waitFor(() => {
        expect(projectsApi.createProject).toHaveBeenCalledWith({
          id: 'new-project',
          name: 'new-project',
          root_path: undefined,
          default_workflow: undefined,
          default_branch: 'main',
          use_git_worktrees: true,
          use_docker_isolation: false,
        })
      })
    })

    it('saving with toggle off sends use_git_worktrees: false', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      vi.mocked(projectsApi.createProject).mockResolvedValue(
        makeProject({ id: 'new-project', use_git_worktrees: false })
      )

      renderPage()

      await screen.findByText('No projects found. Create one to get started.')
      const newButton = screen.getByRole('button', { name: /new project/i })
      await user.click(newButton)

      // Fill required fields
      await user.type(screen.getByPlaceholderText('project-id'), 'new-project')
      await user.type(screen.getByPlaceholderText('main'), 'main')

      // Leave toggle off
      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      expect(toggle).toHaveAttribute('aria-checked', 'false')

      // Create project
      const createButton = screen.getByRole('button', { name: /^create$/i })
      await user.click(createButton)

      await waitFor(() => {
        expect(projectsApi.createProject).toHaveBeenCalledWith({
          id: 'new-project',
          name: 'new-project',
          root_path: undefined,
          default_workflow: undefined,
          default_branch: 'main',
          use_git_worktrees: false,
          use_docker_isolation: false,
        })
      })
    })
  })

  describe('Edit form', () => {
    it('toggle reflects current project value (true)', async () => {
      const user = userEvent.setup()
      const project = makeProject({ use_git_worktrees: true, default_branch: 'main' })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })

      renderPage()

      await screen.findByText('Test Project')
      const editButton = screen.getByRole('button', { name: '' })
      await user.click(editButton)

      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      expect(toggle).toHaveAttribute('aria-checked', 'true')
    })

    it('toggle reflects current project value (false)', async () => {
      const user = userEvent.setup()
      const project = makeProject({ use_git_worktrees: false, default_branch: 'main' })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })

      renderPage()

      await screen.findByText('Test Project')
      const editButton = screen.getByRole('button', { name: '' })
      await user.click(editButton)

      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      expect(toggle).toHaveAttribute('aria-checked', 'false')
    })

    it('toggle disabled when default_branch is empty in existing project', async () => {
      const user = userEvent.setup()
      const project = makeProject({ default_branch: null, use_git_worktrees: false })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })

      renderPage()

      await screen.findByText('Test Project')
      const editButton = screen.getByRole('button', { name: '' })
      await user.click(editButton)

      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      expect(toggle).toBeDisabled()
    })

    it('clearing default_branch in edit mode auto-unchecks toggle', async () => {
      const user = userEvent.setup()
      const project = makeProject({ default_branch: 'main', use_git_worktrees: true })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })

      renderPage()

      await screen.findByText('Test Project')
      const editButton = screen.getByRole('button', { name: '' })
      await user.click(editButton)

      const branchInput = screen.getByDisplayValue('main')
      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })

      // Should start checked
      expect(toggle).toHaveAttribute('aria-checked', 'true')

      // Clear branch field
      await user.clear(branchInput)

      // Toggle should be unchecked and disabled
      expect(toggle).toHaveAttribute('aria-checked', 'false')
      expect(toggle).toBeDisabled()
    })

    it('update request includes use_git_worktrees when true', async () => {
      const user = userEvent.setup()
      const project = makeProject({ default_branch: 'main', use_git_worktrees: false })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })
      vi.mocked(projectsApi.updateProject).mockResolvedValue(
        makeProject({ use_git_worktrees: true })
      )

      renderPage()

      await screen.findByText('Test Project')
      const editButton = screen.getByRole('button', { name: '' })
      await user.click(editButton)

      // Enable toggle
      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      await user.click(toggle)
      expect(toggle).toHaveAttribute('aria-checked', 'true')

      // Save
      const saveButton = screen.getByRole('button', { name: /save/i })
      await user.click(saveButton)

      await waitFor(() => {
        expect(projectsApi.updateProject).toHaveBeenCalledWith('test-project', {
          name: 'Test Project',
          root_path: '/test/path',
          default_workflow: 'feature',
          default_branch: 'main',
          use_git_worktrees: true,
          use_docker_isolation: false,
        })
      })
    })

    it('update request includes use_git_worktrees when false', async () => {
      const user = userEvent.setup()
      const project = makeProject({ default_branch: 'main', use_git_worktrees: true })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })
      vi.mocked(projectsApi.updateProject).mockResolvedValue(
        makeProject({ use_git_worktrees: false })
      )

      renderPage()

      await screen.findByText('Test Project')
      const editButton = screen.getByRole('button', { name: '' })
      await user.click(editButton)

      // Disable toggle
      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      await user.click(toggle)
      expect(toggle).toHaveAttribute('aria-checked', 'false')

      // Save
      const saveButton = screen.getByRole('button', { name: /save/i })
      await user.click(saveButton)

      await waitFor(() => {
        expect(projectsApi.updateProject).toHaveBeenCalledWith('test-project', {
          name: 'Test Project',
          root_path: '/test/path',
          default_workflow: 'feature',
          default_branch: 'main',
          use_git_worktrees: false,
          use_docker_isolation: false,
        })
      })
    })
  })

  describe('Display mode', () => {
    it('shows "Worktrees: enabled" when use_git_worktrees is true', async () => {
      const project = makeProject({ use_git_worktrees: true })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })

      renderPage()

      await screen.findByText('Test Project')
      expect(screen.getByText('Worktrees: enabled')).toBeInTheDocument()
    })

    it('does not show worktrees info when use_git_worktrees is false', async () => {
      const project = makeProject({ use_git_worktrees: false })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })

      renderPage()

      await screen.findByText('Test Project')
      expect(screen.queryByText(/worktrees/i)).not.toBeInTheDocument()
    })

    it('shows worktrees alongside other metadata', async () => {
      const project = makeProject({
        root_path: '/custom/path',
        default_workflow: 'bugfix',
        default_branch: 'develop',
        use_git_worktrees: true,
      })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })

      renderPage()

      await screen.findByText('Test Project')
      expect(screen.getByText('Path: /custom/path')).toBeInTheDocument()
      expect(screen.getByText('Workflow: bugfix')).toBeInTheDocument()
      expect(screen.getByText('Branch: develop')).toBeInTheDocument()
      expect(screen.getByText('Worktrees: enabled')).toBeInTheDocument()
    })
  })
})
