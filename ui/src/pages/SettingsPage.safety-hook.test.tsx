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

async function goToProjectsTab() {
  await userEvent.click(screen.getByRole('button', { name: 'Projects' }))
}

describe('SettingsPage — safety hook display', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows "Safety hook: enabled" when claude_safety_hook is set', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue({
      projects: [makeProject({ claude_safety_hook: '{"enabled":true}' })],
    })
    renderPage()
    await goToProjectsTab()
    await screen.findByText('Test Project')
    expect(screen.getByText('Safety hook: enabled')).toBeInTheDocument()
  })

  it('does not show safety hook indicator when claude_safety_hook is null', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue({
      projects: [makeProject({ claude_safety_hook: null })],
    })
    renderPage()
    await goToProjectsTab()
    await screen.findByText('Test Project')
    expect(screen.queryByText('Safety hook: enabled')).not.toBeInTheDocument()
  })

  it('shows safety hook alongside other metadata', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue({
      projects: [
        makeProject({
          root_path: '/custom/path',
          default_branch: 'develop',
          use_git_worktrees: true,
          claude_safety_hook: '{"enabled":true}',
        }),
      ],
    })
    renderPage()
    await goToProjectsTab()
    await screen.findByText('Test Project')
    expect(screen.getByText('Path: /custom/path')).toBeInTheDocument()
    expect(screen.getByText('Branch: develop')).toBeInTheDocument()
    expect(screen.getByText('Worktrees: enabled')).toBeInTheDocument()
    expect(screen.getByText('Safety hook: enabled')).toBeInTheDocument()
  })
})

describe('SettingsPage — safety hook edit form', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('parses existing safety hook config into form fields on edit', async () => {
    const user = userEvent.setup()
    const hookJson = JSON.stringify({
      enabled: true,
      allow_git: false,
      rm_rf_allowed_paths: ['custom-dir'],
      dangerous_patterns: [],
    })
    vi.mocked(projectsApi.listProjects).mockResolvedValue({
      projects: [makeProject({ claude_safety_hook: hookJson })],
    })
    renderPage()
    await goToProjectsTab()
    await screen.findByText('Test Project')
    await user.click(screen.getByRole('button', { name: '' }))

    expect(screen.getByRole('switch', { name: /enable safety hook/i })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('switch', { name: /allow git operations/i })).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByDisplayValue('custom-dir')).toBeInTheDocument()
  })

  it('populates enable toggle as off when project has no safety hook', async () => {
    const user = userEvent.setup()
    vi.mocked(projectsApi.listProjects).mockResolvedValue({
      projects: [makeProject({ claude_safety_hook: null })],
    })
    renderPage()
    await goToProjectsTab()
    await screen.findByText('Test Project')
    await user.click(screen.getByRole('button', { name: '' }))

    expect(screen.getByRole('switch', { name: /enable safety hook/i })).toHaveAttribute('aria-checked', 'false')
    expect(screen.queryByRole('switch', { name: /allow git operations/i })).not.toBeInTheDocument()
  })

  it('save request includes JSON when safety hook is enabled', async () => {
    const user = userEvent.setup()
    const project = makeProject({ default_branch: 'main' })
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })
    vi.mocked(projectsApi.updateProject).mockResolvedValue(project)

    renderPage()
    await goToProjectsTab()
    await screen.findByText('Test Project')
    await user.click(screen.getByRole('button', { name: '' }))

    await user.click(screen.getByRole('switch', { name: /enable safety hook/i }))
    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      const callArgs = vi.mocked(projectsApi.updateProject).mock.calls[0]
      const updateData = callArgs[1]
      expect(updateData.claude_safety_hook).toBeTruthy()
      const parsed = JSON.parse(updateData.claude_safety_hook!)
      expect(parsed.enabled).toBe(true)
      expect(parsed.allow_git).toBe(true)
      expect(parsed.rm_rf_allowed_paths).toContain('node_modules')
      expect(parsed.dangerous_patterns).toContain('rm -rf /')
      expect(parsed.dangerous_patterns).toContain('DROP TABLE')
    })
  })

  it('does not overwrite existing dangerous patterns when toggling hook off then on', async () => {
    const user = userEvent.setup()
    const hookJson = JSON.stringify({
      enabled: true,
      allow_git: true,
      rm_rf_allowed_paths: [],
      dangerous_patterns: ['my-custom-pattern'],
    })
    const project = makeProject({ claude_safety_hook: hookJson })
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })
    vi.mocked(projectsApi.updateProject).mockResolvedValue(project)

    renderPage()
    await goToProjectsTab()
    await screen.findByText('Test Project')
    await user.click(screen.getByRole('button', { name: '' }))

    const hookToggle = screen.getByRole('switch', { name: /enable safety hook/i })
    await user.click(hookToggle) // disable
    await user.click(hookToggle) // re-enable
    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      const parsed = JSON.parse(
        vi.mocked(projectsApi.updateProject).mock.calls[0][1].claude_safety_hook!
      )
      expect(parsed.dangerous_patterns).toContain('my-custom-pattern')
      expect(parsed.dangerous_patterns).not.toContain('rm -rf /')
    })
  })

  it('save request sends empty string when safety hook is disabled', async () => {
    const user = userEvent.setup()
    const project = makeProject({ default_branch: 'main' })
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })
    vi.mocked(projectsApi.updateProject).mockResolvedValue(project)

    renderPage()
    await goToProjectsTab()
    await screen.findByText('Test Project')
    await user.click(screen.getByRole('button', { name: '' }))

    expect(screen.getByRole('switch', { name: /enable safety hook/i })).toHaveAttribute('aria-checked', 'false')
    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      expect(projectsApi.updateProject).toHaveBeenCalledWith(
        'test-project',
        expect.objectContaining({ claude_safety_hook: '' })
      )
    })
  })
})

describe('SettingsPage — create project safety hook defaults', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [makeProject()] })
  })

  it('shows safety hook enabled by default in create form', async () => {
    const user = userEvent.setup()
    renderPage()
    await goToProjectsTab()
    await screen.findByText('Test Project')
    await user.click(screen.getByRole('button', { name: /new project/i }))

    expect(screen.getByRole('switch', { name: /enable safety hook/i })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('switch', { name: /allow git operations/i })).toHaveAttribute('aria-checked', 'true')
  })

  it('create mutation includes claude_safety_hook with default dangerous patterns', async () => {
    const user = userEvent.setup()
    vi.mocked(projectsApi.createProject).mockResolvedValue(makeProject({ id: 'new-proj' }))

    renderPage()
    await goToProjectsTab()
    await screen.findByText('Test Project')
    await user.click(screen.getByRole('button', { name: /new project/i }))
    await user.type(screen.getByPlaceholderText('project-id'), 'new-proj')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      const createData = vi.mocked(projectsApi.createProject).mock.calls[0][0]
      expect(createData.claude_safety_hook).toBeTruthy()
      const parsed = JSON.parse(createData.claude_safety_hook!)
      expect(parsed.enabled).toBe(true)
      expect(parsed.allow_git).toBe(true)
      expect(parsed.dangerous_patterns).toContain('rm -rf /')
      expect(parsed.dangerous_patterns).toContain('DROP TABLE')
      expect(parsed.rm_rf_allowed_paths).toContain('node_modules')
    })
  })
})
