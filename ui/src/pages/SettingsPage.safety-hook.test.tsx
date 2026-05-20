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

vi.mock('@/api/projectSettings', () => ({
  getArtifactStorage: vi.fn().mockResolvedValue({ mode: 'internal' }),
  setArtifactStorage: vi.fn(),
  getCleanup: vi.fn().mockResolvedValue({ enabled: false, retention_limit: 100 }),
  setCleanup: vi.fn(),
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

describe('SettingsPage — create project safety hook defaults', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [makeProject()] })
  })

  it('shows safety hook disabled by default in create form', async () => {
    const user = userEvent.setup()
    renderPage()
    await goToProjectsTab()
    await screen.findByText('Test Project')
    await user.click(screen.getByRole('button', { name: /new project/i }))

    expect(screen.getByRole('switch', { name: /enable safety hook/i })).toHaveAttribute('aria-checked', 'false')
    expect(screen.queryByRole('switch', { name: /allow git operations/i })).not.toBeInTheDocument()
  })

  it('create mutation omits claude_safety_hook when safety hook is disabled by default', async () => {
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
      expect(createData.claude_safety_hook).toBeUndefined()
    })
  })
})
