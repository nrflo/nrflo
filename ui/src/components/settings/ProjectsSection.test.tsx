import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ProjectsSection } from './ProjectsSection'
import * as projectsApi from '@/api/projects'
import { renderWithQuery } from '@/test/utils'
import type { Project } from '@/api/projects'

vi.mock('@/api/projects')

const mockNavigate = vi.fn()
vi.mock('react-router-dom', async () => ({
  ...(await vi.importActual('react-router-dom')),
  useNavigate: () => mockNavigate,
}))

const mockSetCurrentProject = vi.fn()
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn(() => ({
    currentProject: 'aveva',
    setCurrentProject: mockSetCurrentProject,
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
  Element.prototype.scrollIntoView = vi.fn()
})

describe('ProjectsSection — gear button navigation', () => {
  it('calls setCurrentProject then navigates to /project-settings when gear button clicked', async () => {
    const user = userEvent.setup()
    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection />
      </MemoryRouter>
    )
    await screen.findByText('AVEVA Project')
    await user.click(screen.getByRole('button', { name: 'Settings' }))
    expect(mockSetCurrentProject).toHaveBeenCalledWith('aveva')
    expect(mockNavigate).toHaveBeenCalledWith('/project-settings')
  })
})

describe('ProjectsSection — no env vars editor', () => {
  it('does not render Environment Variables editor in Projects tab', async () => {
    const user = userEvent.setup()
    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection />
      </MemoryRouter>
    )
    await screen.findByText('AVEVA Project')
    await user.click(screen.getByRole('button', { name: /new project/i }))
    expect(screen.queryByText('Environment Variables')).not.toBeInTheDocument()
  })
})

describe('ProjectsSection — create flow', () => {
  it('shows create form when New Project is clicked', async () => {
    const user = userEvent.setup()
    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection />
      </MemoryRouter>
    )
    await screen.findByText('AVEVA Project')
    await user.click(screen.getByRole('button', { name: /new project/i }))
    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
  })

  it('calls createProject when form is submitted with an id', async () => {
    vi.mocked(projectsApi.createProject).mockResolvedValue(
      makeProject({ id: 'new-proj', name: 'New Project' })
    )
    const user = userEvent.setup()
    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection />
      </MemoryRouter>
    )
    await screen.findByText('AVEVA Project')
    await user.click(screen.getByRole('button', { name: /new project/i }))
    await user.type(screen.getByPlaceholderText('project-id'), 'new-proj')
    await user.click(screen.getByRole('button', { name: /create/i }))
    await waitFor(() =>
      expect(projectsApi.createProject).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'new-proj' })
      )
    )
  })
})

describe('ProjectsSection — delete flow', () => {
  beforeEach(() => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue({
      projects: [
        makeProject(),
        makeProject({ id: 'other', name: 'Other Project' }),
      ],
    })
  })

  it('shows delete confirm when delete button is clicked', async () => {
    const user = userEvent.setup()
    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection />
      </MemoryRouter>
    )
    await screen.findByText('AVEVA Project')
    await user.click(screen.getAllByRole('button', { name: 'Delete' })[0])
    expect(screen.getByText(/are you sure/i)).toBeInTheDocument()
  })

  it('cancels delete confirm', async () => {
    const user = userEvent.setup()
    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection />
      </MemoryRouter>
    )
    await screen.findByText('AVEVA Project')
    await user.click(screen.getAllByRole('button', { name: 'Delete' })[0])
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() =>
      expect(screen.queryByText(/are you sure/i)).not.toBeInTheDocument()
    )
    expect(screen.getByText('AVEVA Project')).toBeInTheDocument()
  })

  it('calls deleteProject when delete is confirmed', async () => {
    vi.mocked(projectsApi.deleteProject).mockResolvedValue({ message: 'deleted' })
    const user = userEvent.setup()
    renderWithQuery(
      <MemoryRouter>
        <ProjectsSection />
      </MemoryRouter>
    )
    await screen.findByText('AVEVA Project')
    // open confirm for first project
    await user.click(screen.getAllByRole('button', { name: 'Delete' })[0])
    await screen.findByText(/are you sure/i)
    // confirm delete — destructive button is first in DOM order
    await user.click(screen.getAllByRole('button', { name: 'Delete' })[0])
    await waitFor(() =>
      expect(projectsApi.deleteProject).toHaveBeenCalledWith('aveva')
    )
  })
})
