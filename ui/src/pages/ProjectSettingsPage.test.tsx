import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ProjectSettingsPage } from './ProjectSettingsPage'
import * as projectsApi from '@/api/projects'
import * as envVarsApi from '@/api/projectEnvVars'
import * as catalogHook from '@/hooks/useEnvVarCatalog'
import * as settingsApi from '@/api/projectSettings'
import { renderWithQuery } from '@/test/utils'
import type { Project } from '@/api/projects'

vi.mock('@/api/projects')
vi.mock('@/api/projectEnvVars')
vi.mock('@/hooks/useEnvVarCatalog')
vi.mock('@/api/projectSettings')
vi.mock('@/api/cliModels', () => ({ listCLIModels: vi.fn().mockResolvedValue([]) }))

const mockUseProjectStore = vi.fn()
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (...args: unknown[]) => mockUseProjectStore(...args),
}))

function makeProject(overrides: Partial<Project> = {}): Project {
  return {
    id: 'proj1',
    name: 'Test Project',
    root_path: '/test',
    default_branch: null,
    use_git_worktrees: false,
    push_after_merge: false,
    claude_safety_hook: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeStoreState(overrides: {
  currentProject?: string
  projects?: Project[]
  projectsLoaded?: boolean
} = {}) {
  const state = {
    currentProject: 'proj1',
    projects: [makeProject()],
    projectsLoaded: true,
    loadProjects: vi.fn(),
    setCurrentProject: vi.fn(),
    ...overrides,
  }
  return state
}

beforeEach(() => {
  vi.clearAllMocks()

  mockUseProjectStore.mockImplementation((selector?: (s: ReturnType<typeof makeStoreState>) => unknown) => {
    const state = makeStoreState()
    return typeof selector === 'function' ? selector(state) : state
  })

  vi.mocked(projectsApi.updateProject).mockResolvedValue(makeProject())
  vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [makeProject()] })
  vi.mocked(envVarsApi.listEnvVars).mockResolvedValue([])
  vi.mocked(catalogHook.useEnvVarCatalog).mockReturnValue({ data: [], isLoading: false } as ReturnType<typeof catalogHook.useEnvVarCatalog>)
  vi.mocked(settingsApi.getArtifactStorage).mockResolvedValue({ mode: 'internal' })
  vi.mocked(settingsApi.getCleanup).mockResolvedValue({ enabled: false, retention_limit: 0 })
  vi.mocked(settingsApi.getObserver).mockResolvedValue({ system_context: '', provider: 'anthropic', model: 'claude-sonnet-4-5-20251001' })
  Element.prototype.scrollIntoView = vi.fn()
})

function renderPage() {
  return renderWithQuery(
    <MemoryRouter>
      <ProjectSettingsPage />
    </MemoryRouter>
  )
}

describe('ProjectSettingsPage — active project', () => {
  it('renders project name and Environment Variables editor', async () => {
    renderPage()
    expect(await screen.findByText('Test Project')).toBeInTheDocument()
    expect(screen.getByText('Environment Variables')).toBeInTheDocument()
  })

  it('renders Project Settings card heading', async () => {
    renderPage()
    expect(await screen.findByText('Project Settings')).toBeInTheDocument()
  })

  it('clicking Save calls updateProject with the current project id', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('Environment Variables')
    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => expect(projectsApi.updateProject).toHaveBeenCalledOnce())
    expect(projectsApi.updateProject).toHaveBeenCalledWith('proj1', expect.objectContaining({ name: 'Test Project' }))
  })
})

describe('ProjectSettingsPage — no active project', () => {
  it('shows empty state when currentProject is empty string', () => {
    mockUseProjectStore.mockImplementation((selector?: (s: ReturnType<typeof makeStoreState>) => unknown) => {
      const state = makeStoreState({ currentProject: '', projects: [makeProject()] })
      return typeof selector === 'function' ? selector(state) : state
    })
    renderPage()
    expect(screen.getByText('No active project selected.')).toBeInTheDocument()
    expect(screen.queryByText('Environment Variables')).not.toBeInTheDocument()
  })

  it('shows empty state when no matching project found', () => {
    mockUseProjectStore.mockImplementation((selector?: (s: ReturnType<typeof makeStoreState>) => unknown) => {
      const state = makeStoreState({ currentProject: 'unknown-id', projects: [] })
      return typeof selector === 'function' ? selector(state) : state
    })
    renderPage()
    expect(screen.getByText('No active project selected.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /save/i })).not.toBeInTheDocument()
  })
})

describe('ProjectSettingsPage — loading state', () => {
  it('shows loading message when projectsLoaded is false', () => {
    mockUseProjectStore.mockImplementation((selector?: (s: ReturnType<typeof makeStoreState>) => unknown) => {
      const state = makeStoreState({ projectsLoaded: false })
      return typeof selector === 'function' ? selector(state) : state
    })
    renderPage()
    expect(screen.getByText('Loading project settings...')).toBeInTheDocument()
    expect(screen.queryByText('Environment Variables')).not.toBeInTheDocument()
  })
})
