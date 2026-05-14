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

vi.mock('@/components/settings/ProvidersSection', () => ({
  ProvidersSection: ({ activeProvider }: { activeProvider: string }) => (
    <div data-testid="providers-section" data-provider={activeProvider}>ProvidersSection</div>
  ),
}))

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
    projects: [{ id: 'p1' }],
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

describe('SettingsPage — Providers tab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setStore()
  })

  it('renders ProvidersSection with claude as default when no sub param', () => {
    renderPage('/settings?tab=providers')
    const section = screen.getByTestId('providers-section')
    expect(section).toBeInTheDocument()
    expect(section).toHaveAttribute('data-provider', 'claude')
  })

  it('renders ProvidersSection with opencode when sub=opencode', () => {
    renderPage('/settings?tab=providers&sub=opencode')
    expect(screen.getByTestId('providers-section')).toHaveAttribute('data-provider', 'opencode')
  })

  it('renders ProvidersSection with codex when sub=codex', () => {
    renderPage('/settings?tab=providers&sub=codex')
    expect(screen.getByTestId('providers-section')).toHaveAttribute('data-provider', 'codex')
  })

  it('clicking Providers tab shows ProvidersSection with claude sub-tab', async () => {
    renderPage('/settings')
    await userEvent.setup().click(screen.getByRole('button', { name: 'Providers' }))
    expect(await screen.findByTestId('providers-section')).toHaveAttribute('data-provider', 'claude')
  })

  it('clicking OpenCode sub-tab changes provider to opencode', async () => {
    renderPage('/settings?tab=providers')
    await userEvent.setup().click(screen.getByRole('button', { name: 'OpenCode' }))
    expect(screen.getByTestId('providers-section')).toHaveAttribute('data-provider', 'opencode')
  })

  it('clicking Codex sub-tab changes provider to codex', async () => {
    renderPage('/settings?tab=providers')
    await userEvent.setup().click(screen.getByRole('button', { name: 'Codex' }))
    expect(screen.getByTestId('providers-section')).toHaveAttribute('data-provider', 'codex')
  })

  it('clicking Claude sub-tab keeps provider as claude', async () => {
    renderPage('/settings?tab=providers&sub=opencode')
    await userEvent.setup().click(screen.getByRole('button', { name: 'Claude' }))
    expect(screen.getByTestId('providers-section')).toHaveAttribute('data-provider', 'claude')
  })

  it('provider sub-tabs are not visible when general tab is active', () => {
    renderPage('/settings')
    expect(screen.queryByRole('button', { name: 'OpenCode' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Codex' })).not.toBeInTheDocument()
  })

  it('ProvidersSection is not rendered when a different tab is active', () => {
    renderPage('/settings?tab=general')
    expect(screen.queryByTestId('providers-section')).not.toBeInTheDocument()
  })
})
