import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SettingsPage } from './SettingsPage'
import * as projectsApi from '@/api/projects'

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

vi.mock('@/hooks/useTiering', () => ({
  useTieringReport: vi.fn(() => ({ isLoading: false, error: null, data: { projects: [], markdown: '' } })),
  useApplyTiering: vi.fn(() => ({ mutate: vi.fn(), isPending: false, isError: false, error: null, variables: undefined })),
}))

vi.mock('@/api/settings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/settings')>()
  return {
    ...actual,
    getGlobalSettings: vi.fn().mockResolvedValue({
      menu_new_ticket: false, menu_import_spec: false, menu_git: true,
      menu_chain_executions: true, menu_schedules: false, menu_workflow_chains: false,
      menu_python_scripts: false, menu_documentation: true, menu_errors: false,
      menu_agent_sessions: false,
    }),
    updateGlobalSettings: vi.fn().mockResolvedValue(undefined),
  }
})

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

describe('SettingsPage - Tiering tab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
  })

  it('Tiering tab button is present and selecting it renders TieringSection', async () => {
    const user = userEvent.setup()
    renderPage()

    const tieringTab = screen.getByRole('button', { name: 'Tiering' })
    expect(tieringTab).toBeInTheDocument()

    await user.click(tieringTab)
    expect(await screen.findByText('Tiering', { selector: 'h2' })).toBeInTheDocument()
    expect(screen.getByText('No projects to report on.')).toBeInTheDocument()
  })
})
