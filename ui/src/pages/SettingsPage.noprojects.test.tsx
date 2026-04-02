import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SettingsPage } from './SettingsPage'

const mockUseProjectStore = vi.fn()

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector?: (s: unknown) => unknown) => mockUseProjectStore(selector),
}))

vi.mock('@/api/projects', async () => {
  const actual = await vi.importActual('@/api/projects')
  return {
    ...actual,
    listProjects: vi.fn().mockResolvedValue({ projects: [] }),
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

vi.mock('@/hooks/useLogs')

type StoreShape = {
  currentProject: string
  setCurrentProject: (id: string) => void
  loadProjects: () => void
  projects: { id: string }[]
  projectsLoaded: boolean
}

function setStore(overrides: Partial<StoreShape> = {}) {
  const store: StoreShape = {
    currentProject: '',
    setCurrentProject: vi.fn(),
    loadProjects: vi.fn(),
    projects: [],
    projectsLoaded: true,
    ...overrides,
  }
  mockUseProjectStore.mockImplementation((selector?: (s: StoreShape) => unknown) =>
    selector ? selector(store) : store
  )
}

function renderPage(initialUrl = '/settings') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialUrl]}>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SettingsPage — no projects warning banner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setStore()
  })

  it('shows banner when no projects are configured', () => {
    renderPage()
    expect(screen.getByText(/No projects configured/)).toBeInTheDocument()
  })

  it('hides banner after clicking dismiss', async () => {
    const user = userEvent.setup()
    renderPage()
    expect(screen.getByText(/No projects configured/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(screen.queryByText(/No projects configured/)).not.toBeInTheDocument()
  })

  it('does not show banner when projects exist', () => {
    setStore({ projects: [{ id: 'existing-project' }] })
    renderPage()
    expect(screen.queryByText(/No projects configured/)).not.toBeInTheDocument()
  })
})

describe('SettingsPage — tab URL param', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setStore({ projects: [{ id: 'p1' }] })
  })

  it('activates Projects tab when URL has ?tab=projects', async () => {
    renderPage('/settings?tab=projects')
    // ProjectsSection renders when Projects tab is active — wait for its content
    await screen.findByRole('button', { name: /new project/i })
  })

  it('defaults to General tab when no tab param', () => {
    renderPage('/settings')
    // General tab button is present and Settings heading is shown
    expect(screen.getByRole('button', { name: 'General' })).toBeInTheDocument()
    // ProjectsSection content should NOT be rendered
    expect(screen.queryByRole('button', { name: /new project/i })).not.toBeInTheDocument()
  })

  it('defaults to General tab when tab param is invalid', () => {
    renderPage('/settings?tab=invalid-tab')
    expect(screen.queryByRole('button', { name: /new project/i })).not.toBeInTheDocument()
  })
})
