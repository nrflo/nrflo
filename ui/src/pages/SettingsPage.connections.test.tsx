import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SettingsPage } from './SettingsPage'
import type { Connection } from '@/stores/connectionsStore'

const LOCAL: Connection = { id: 'local', name: 'Local', baseURL: '', isLocal: true }

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector?: (s: unknown) => unknown) => {
    const store = {
      currentProject: 'proj-1',
      setCurrentProject: vi.fn(),
      loadProjects: vi.fn(),
      projects: [{ id: 'proj-1' }],
      projectsLoaded: true,
    }
    return selector ? selector(store) : store
  },
}))

vi.mock('@/stores/connectionsStore', () => ({
  useConnectionsStore: vi.fn((selector?: (s: unknown) => unknown) => {
    const store = {
      list: [LOCAL],
      activeId: 'local',
      add: vi.fn(),
      remove: vi.fn(),
      setActive: vi.fn(),
    }
    return selector ? selector(store) : store
  }),
}))

vi.mock('@/api/client', () => ({
  testConnection: vi.fn(),
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

vi.mock('@/hooks/useLogs', () => ({
  useLogs: vi.fn().mockReturnValue({
    data: { lines: [], type: 'be' },
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
}))

function renderPage(initialEntries: string[] = ['/']) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SettingsPage - Connections tab', () => {
  beforeEach(() => vi.clearAllMocks())

  it('Connections tab button is present', () => {
    renderPage()
    expect(screen.getByRole('button', { name: 'Connections' })).toBeInTheDocument()
  })

  it('clicking Connections tab renders the Connections section heading', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: 'Connections' }))

    expect(screen.getByRole('heading', { name: /^connections$/i })).toBeInTheDocument()
  })

  it('clicking Connections tab renders the connections table', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: 'Connections' }))

    expect(screen.getByText('Name')).toBeInTheDocument()
    expect(screen.getByText('Base URL')).toBeInTheDocument()
    expect(screen.getByText('Local')).toBeInTheDocument()
  })

  it('deep-link ?tab=connections renders ConnectionsSection on first render', () => {
    renderPage(['/?tab=connections'])

    expect(screen.getByRole('heading', { name: /^connections$/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /add connection/i })).toBeInTheDocument()
  })

  it('Connections tab does not render ConnectionsSection by default', () => {
    renderPage()
    expect(screen.queryByRole('heading', { name: /^connections$/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /add connection/i })).not.toBeInTheDocument()
  })
})
