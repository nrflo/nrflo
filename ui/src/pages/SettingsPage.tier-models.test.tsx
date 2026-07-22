import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SettingsPage } from './SettingsPage'
import * as projectsApi from '@/api/projects'
import * as tierModelsApi from '@/api/tierModels'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector?: (s: { currentProject: string; setCurrentProject: () => void; loadProjects: () => void; projects: unknown[]; projectsLoaded: boolean }) => unknown) => {
    const store = {
      currentProject: 'test-project',
      setCurrentProject: vi.fn(),
      loadProjects: vi.fn(),
      projects: [{ id: 'test-project' }],
      projectsLoaded: true,
    }
    return selector ? selector(store) : store
  },
}))

vi.mock('@/api/projects', async () => {
  const actual = await vi.importActual('@/api/projects')
  return { ...actual, listProjects: vi.fn() }
})

vi.mock('@/api/tierModels')

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/']}>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SettingsPage - Tier Models tab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
    vi.mocked(tierModelsApi.listTierModels).mockResolvedValue([])
  })

  it('Tier Models tab button is present and selecting it renders TierModelsSection', async () => {
    const user = userEvent.setup()
    renderPage()

    const tab = screen.getByRole('button', { name: 'Tier Models' })
    expect(tab).toBeInTheDocument()

    await user.click(tab)
    expect(await screen.findByText('Tier Models', { selector: 'h2' })).toBeInTheDocument()
    expect(screen.getByText('Tier 1')).toBeInTheDocument()
    expect(screen.getByText('Tier 5')).toBeInTheDocument()
  })
})
