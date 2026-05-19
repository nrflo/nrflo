import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SettingsPage } from './SettingsPage'
import * as projectsApi from '@/api/projects'
import type { Project } from '@/api/projects'
import * as logsHook from '@/hooks/useLogs'
import * as settingsApi from '@/api/settings'
import type { GlobalSettings } from '@/api/settings'

const mockSetCurrentProject = vi.fn()
const mockLoadProjects = vi.fn()

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector?: (s: { currentProject: string; setCurrentProject: (id: string) => void; loadProjects: () => void; projects: unknown[]; projectsLoaded: boolean }) => unknown) => {
    const store = {
      currentProject: 'test-project',
      setCurrentProject: mockSetCurrentProject,
      loadProjects: mockLoadProjects,
      projects: [{ id: 'test-project' }],
      projectsLoaded: true,
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

vi.mock('@/api/projectSettings', () => ({
  getArtifactStorage: vi.fn().mockResolvedValue({ mode: 'internal' }),
  setArtifactStorage: vi.fn(),
  getCleanup: vi.fn().mockResolvedValue({ enabled: false, retention_limit: 100 }),
  setCleanup: vi.fn(),
}))

vi.mock('@/hooks/useLogs')

vi.mock('@/api/settings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/settings')>()
  return {
    ...actual,
    getGlobalSettings: vi.fn().mockResolvedValue({
      menu_new_ticket: false, menu_import_spec: false, menu_git: true,
      menu_chain_executions: true, menu_schedules: false, menu_workflow_chains: false,
      menu_python_scripts: false, menu_documentation: true, menu_errors: false,
      menu_agent_sessions: false,
    }),
    updateGlobalSettings: vi.fn().mockResolvedValue(undefined),
  }
})

function makeProject(overrides: Partial<Project> = {}): Project {
  return {
    id: 'test-project',
    name: 'Test Project',
    root_path: '/test/path',
    default_branch: 'main',
    use_git_worktrees: false,
    push_after_merge: false,
    claude_safety_hook: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeSettings(overrides: Partial<GlobalSettings> = {}): GlobalSettings {
  return {
    menu_new_ticket: false,
    menu_import_spec: false,
    menu_git: true,
    menu_chain_executions: true,
    menu_schedules: false,
    menu_workflow_chains: false,
    menu_python_scripts: false,
    menu_documentation: true,
    menu_errors: false,
    menu_agent_sessions: false,
    ...overrides,
  } as GlobalSettings
}

function renderPage(initialEntries: string[] = ['/']) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

async function goToProjectsTab() {
  await userEvent.click(screen.getByRole('button', { name: 'Projects' }))
}

describe('SettingsPage - use_git_worktrees toggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('Create form', () => {
    it('toggle defaults to off', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      renderPage()
      await goToProjectsTab()

      await screen.findByText('No projects found. Create one to get started.')
      const newButton = screen.getByRole('button', { name: /new project/i })
      await userEvent.click(newButton)

      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      expect(toggle).toHaveAttribute('aria-checked', 'false')
    })

    it('toggle is disabled when default_branch is empty', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      renderPage()
      await goToProjectsTab()

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
      await goToProjectsTab()

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
      await goToProjectsTab()

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
      await goToProjectsTab()

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
        expect(projectsApi.createProject).toHaveBeenCalledWith(
          expect.objectContaining({
            id: 'new-project',
            name: 'new-project',
            default_branch: 'main',
            use_git_worktrees: true,
          })
        )
      })
    })

    it('saving with toggle off sends use_git_worktrees: false', async () => {
      const user = userEvent.setup()
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
      vi.mocked(projectsApi.createProject).mockResolvedValue(
        makeProject({ id: 'new-project', use_git_worktrees: false })
      )

      renderPage()
      await goToProjectsTab()

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
        expect(projectsApi.createProject).toHaveBeenCalledWith(
          expect.objectContaining({
            id: 'new-project',
            name: 'new-project',
            default_branch: 'main',
            use_git_worktrees: false,
          })
        )
      })
    })
  })

  describe('Edit form', () => {
    it('toggle reflects current project value (true)', async () => {
      const user = userEvent.setup()
      const project = makeProject({ use_git_worktrees: true, default_branch: 'main' })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })

      renderPage()
      await goToProjectsTab()

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
      await goToProjectsTab()

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
      await goToProjectsTab()

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
      await goToProjectsTab()

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
      await goToProjectsTab()

      await screen.findByText('Test Project')
      const editButton = screen.getByRole('button', { name: '' })
      await user.click(editButton)

      // Enable toggle
      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      await user.click(toggle)
      expect(toggle).toHaveAttribute('aria-checked', 'true')

      // Save — use last match; editors add their own Save buttons before the form's
      const saveButtons = screen.getAllByRole('button', { name: /save/i })
      await user.click(saveButtons[saveButtons.length - 1])

      await waitFor(() => {
        expect(projectsApi.updateProject).toHaveBeenCalledWith('test-project', {
          name: 'Test Project',
          root_path: '/test/path',
          default_branch: 'main',
          use_git_worktrees: true,
          push_after_merge: false,
          claude_safety_hook: '',
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
      await goToProjectsTab()

      await screen.findByText('Test Project')
      const editButton = screen.getByRole('button', { name: '' })
      await user.click(editButton)

      // Disable toggle
      const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
      await user.click(toggle)
      expect(toggle).toHaveAttribute('aria-checked', 'false')

      // Save — use last match; editors add their own Save buttons before the form's
      const saveButtons = screen.getAllByRole('button', { name: /save/i })
      await user.click(saveButtons[saveButtons.length - 1])

      await waitFor(() => {
        expect(projectsApi.updateProject).toHaveBeenCalledWith('test-project', {
          name: 'Test Project',
          root_path: '/test/path',
          default_branch: 'main',
          use_git_worktrees: false,
          push_after_merge: false,
          claude_safety_hook: '',
        })
      })
    })
  })

  describe('Display mode', () => {
    it('shows "Worktrees: enabled" when use_git_worktrees is true', async () => {
      const project = makeProject({ use_git_worktrees: true })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })

      renderPage()
      await goToProjectsTab()

      await screen.findByText('Test Project')
      expect(screen.getByText('Worktrees: enabled')).toBeInTheDocument()
    })

    it('does not show worktrees info when use_git_worktrees is false', async () => {
      const project = makeProject({ use_git_worktrees: false })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })

      renderPage()
      await goToProjectsTab()

      await screen.findByText('Test Project')
      expect(screen.queryByText(/worktrees/i)).not.toBeInTheDocument()
    })

    it('shows worktrees alongside other metadata', async () => {
      const project = makeProject({
        root_path: '/custom/path',
        default_branch: 'develop',
        use_git_worktrees: true,
      })
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })

      renderPage()
      await goToProjectsTab()

      await screen.findByText('Test Project')
      expect(screen.getByText('Path: /custom/path')).toBeInTheDocument()
      expect(screen.getByText('Branch: develop')).toBeInTheDocument()
      expect(screen.getByText('Worktrees: enabled')).toBeInTheDocument()
    })
  })
})

describe('SettingsPage - Logs tab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
    vi.mocked(logsHook.useLogs).mockReturnValue({
      data: { lines: [], type: 'be' },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as unknown as ReturnType<typeof logsHook.useLogs>)
  })

  it('Logs tab button is present as the last tab', () => {
    renderPage()
    const logsTab = screen.getByRole('button', { name: 'Logs' })
    expect(logsTab).toBeInTheDocument()
    const allTabs = screen.getAllByRole('button').filter((b) =>
      ['General', 'Menu Panel', 'Projects', 'System Agents', 'Default Templates', 'CLI Models', 'Logs'].includes(b.textContent ?? '')
    )
    expect(allTabs[allTabs.length - 1]).toHaveTextContent('Logs')
  })

  it('clicking Logs tab shows filter input with no BE/FE sub-tab buttons', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Logs' }))
    expect(screen.getByPlaceholderText('Filter logs...')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /BE/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /FE/ })).not.toBeInTheDocument()
  })
})

describe('SettingsPage - Menu Panel tab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    vi.mocked(logsHook.useLogs).mockReturnValue({
      data: { lines: [], type: 'be' },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as unknown as ReturnType<typeof logsHook.useLogs>)
  })

  it('clicking Menu Panel tab renders the Menu Panel card heading', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Menu Panel' }))
    expect(await screen.findByText('Menu Panel', { selector: 'h3' })).toBeInTheDocument()
  })

  it('default General tab does not render the Menu Panel card heading', async () => {
    renderPage()
    // Wait for the page to settle on the General tab
    await screen.findByRole('button', { name: 'Menu Panel' })
    expect(screen.queryByText('Menu Panel', { selector: 'h3' })).not.toBeInTheDocument()
  })

  it('deep-link ?tab=menu-panel renders the Menu Panel card on initial mount', async () => {
    renderPage(['/?tab=menu-panel'])
    expect(await screen.findByText('Menu Panel', { selector: 'h3' })).toBeInTheDocument()
  })
})
