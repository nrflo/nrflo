import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProjectsSection } from './ProjectsSection'
import * as projectsApi from '@/api/projects'
import { renderWithQuery } from '@/test/utils'
import type { Project } from '@/api/projects'

vi.mock('@/api/projects')
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn(() => ({
    currentProject: 'proj-1',
    setCurrentProject: vi.fn(),
    loadProjects: vi.fn(),
  })),
}))

function makeProject(overrides: Partial<Project> = {}): Project {
  return {
    id: 'proj-1',
    name: 'Project Alpha',
    root_path: '/home/user/alpha',
    default_branch: 'main',
    use_git_worktrees: false,
    push_after_merge: false,
    interactive_cli_mode: false,
    claude_safety_hook: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ProjectsSection — interactive_cli_mode display', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows "Interactive CLI: enabled" in project summary when set', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue({
      projects: [makeProject({ interactive_cli_mode: true })],
    })
    renderWithQuery(<ProjectsSection />)
    expect(await screen.findByText(/Interactive CLI: enabled/)).toBeInTheDocument()
  })

  it('omits "Interactive CLI: enabled" when false', async () => {
    vi.mocked(projectsApi.listProjects).mockResolvedValue({
      projects: [makeProject({ interactive_cli_mode: false })],
    })
    renderWithQuery(<ProjectsSection />)
    await screen.findByText('Project Alpha')
    expect(screen.queryByText(/Interactive CLI: enabled/)).not.toBeInTheDocument()
  })
})

describe('ProjectsSection — interactive_cli_mode edit + save', () => {
  beforeEach(() => vi.clearAllMocks())

  it('edit-load hydrates toggle from project and PATCH includes interactive_cli_mode when toggled', async () => {
    const project = makeProject({ interactive_cli_mode: false })
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })
    vi.mocked(projectsApi.updateProject).mockResolvedValue({
      ...project,
      interactive_cli_mode: true,
    })

    const user = userEvent.setup()
    renderWithQuery(<ProjectsSection />)
    await screen.findByText('Project Alpha')

    // buttons order: [0] New Project, [1] Edit (pencil icon), [2] Delete (trash)
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[1])

    // edit form opened; toggle starts unchecked (project has interactive_cli_mode: false)
    const toggle = await screen.findByRole('switch', { name: /interactive cli mode/i })
    expect(toggle).toHaveAttribute('aria-checked', 'false')

    // toggle to true
    await user.click(toggle)

    // save
    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      expect(projectsApi.updateProject).toHaveBeenCalledWith(
        'proj-1',
        expect.objectContaining({ interactive_cli_mode: true })
      )
    })
  })

  it('edit-load starts toggled when project.interactive_cli_mode is true', async () => {
    const project = makeProject({ interactive_cli_mode: true })
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [project] })
    vi.mocked(projectsApi.updateProject).mockResolvedValue(project)

    const user = userEvent.setup()
    renderWithQuery(<ProjectsSection />)
    await screen.findByText('Project Alpha')

    const buttons = screen.getAllByRole('button')
    await user.click(buttons[1])

    const toggle = await screen.findByRole('switch', { name: /interactive cli mode/i })
    expect(toggle).toHaveAttribute('aria-checked', 'true')
  })
})
