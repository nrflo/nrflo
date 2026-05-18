import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ProjectsSection } from './ProjectsSection'
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
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn(() => ({
    currentProject: 'aveva',
    setCurrentProject: vi.fn(),
    loadProjects: vi.fn(),
  })),
}))

function makeProject(overrides: Partial<Project> = {}): Project {
  return {
    id: 'aveva',
    name: 'AVEVA Project',
    root_path: null,
    default_branch: null,
    use_git_worktrees: false,
    push_after_merge: false,
    claude_safety_hook: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [makeProject()] })
  vi.mocked(envVarsApi.listEnvVars).mockResolvedValue([])
  vi.mocked(catalogHook.useEnvVarCatalog).mockReturnValue({ data: [], isLoading: false } as any)
  vi.mocked(settingsApi.getArtifactStorage).mockResolvedValue({ mode: 'internal' })
  vi.mocked(settingsApi.getCleanup).mockResolvedValue({ enabled: false, retention_limit: 0 })
  Element.prototype.scrollIntoView = vi.fn()
})

describe('ProjectsSection — unified save flow', () => {
  beforeEach(() => {
    vi.mocked(projectsApi.updateProject).mockResolvedValue(makeProject())
    vi.mocked(settingsApi.setArtifactStorage).mockResolvedValue({ mode: 'internal' })
    vi.mocked(settingsApi.setCleanup).mockResolvedValue({ enabled: false, retention_limit: 0 })
  })

  it('Save with unmodified subforms calls only updateProject', async () => {
    const user = userEvent.setup()
    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection initialEditProjectId="aveva" />
      </MemoryRouter>
    )

    await screen.findByText('Environment Variables')
    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() =>
      expect(screen.queryByText('Environment Variables')).not.toBeInTheDocument()
    )
    expect(projectsApi.updateProject).toHaveBeenCalledOnce()
    expect(settingsApi.setArtifactStorage).not.toHaveBeenCalled()
    expect(settingsApi.setCleanup).not.toHaveBeenCalled()
  })

  it('setArtifactStorage rejection shows inline error and keeps form open', async () => {
    const user = userEvent.setup()
    vi.mocked(settingsApi.setArtifactStorage).mockRejectedValue(new Error('bucket required'))

    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection initialEditProjectId="aveva" />
      </MemoryRouter>
    )

    await screen.findByText('Environment Variables')

    await user.click(screen.getByRole('button', { name: /internal/i }))
    await user.click(screen.getByText('Cloudflare R2'))

    await user.click(screen.getByRole('button', { name: /save/i }))

    await screen.findByText('bucket required')
    expect(screen.getByText('Environment Variables')).toBeInTheDocument()
    expect(settingsApi.setArtifactStorage).toHaveBeenCalledOnce()
  })
})

describe('ProjectsSection — initialEditProjectId', () => {
  it('auto-opens ProjectForm for matching project on load', async () => {
    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection initialEditProjectId="aveva" />
      </MemoryRouter>
    )

    // ProjectEnvVarsEditor renders "Environment Variables" when edit form opens
    expect(await screen.findByText('Environment Variables')).toBeInTheDocument()
  })

  it('does not auto-open edit form when id does not match any project', async () => {
    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection initialEditProjectId="no-such-project" />
      </MemoryRouter>
    )

    await screen.findByText('AVEVA Project')
    expect(screen.queryByText('Environment Variables')).not.toBeInTheDocument()
  })

  it('renders project list without edit form when initialEditProjectId is absent', async () => {
    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection />
      </MemoryRouter>
    )

    await screen.findByText('AVEVA Project')
    expect(screen.queryByText('Environment Variables')).not.toBeInTheDocument()
  })
})
