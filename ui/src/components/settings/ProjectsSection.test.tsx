import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ProjectsSection } from './ProjectsSection'
import * as projectsApi from '@/api/projects'
import * as envVarsApi from '@/api/projectEnvVars'
import * as catalogHook from '@/hooks/useEnvVarCatalog'
import { renderWithQuery } from '@/test/utils'
import type { Project } from '@/api/projects'

vi.mock('@/api/projects')
vi.mock('@/api/projectEnvVars')
vi.mock('@/hooks/useEnvVarCatalog')
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
  Element.prototype.scrollIntoView = vi.fn()
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
