import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SettingsPage } from './SettingsPage'
import * as projectsApi from '@/api/projects'
import * as systemAgentRunsApi from '@/api/systemAgentRuns'

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

vi.mock('@/api/systemAgentDefs', () => ({
  listSystemAgentDefs: vi.fn().mockResolvedValue([]),
  createSystemAgentDef: vi.fn(),
  updateSystemAgentDef: vi.fn(),
  deleteSystemAgentDef: vi.fn(),
}))

vi.mock('@/api/systemAgentRuns')
vi.mock('@/api/handoffDigest', () => ({
  fetchSessionHandoffDigest: vi.fn().mockResolvedValue(null),
}))

vi.mock('@/providers/WebSocketProvider', () => ({
  useWebSocketContext: () => ({
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }),
}))

function renderPage(initialEntries: string[] = ['/']) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SettingsPage - System Agents sub-tabs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })
    vi.mocked(systemAgentRunsApi.listSystemAgentRuns).mockResolvedValue({ items: [], limit: 50 })
  })

  it('defaults to the Definitions sub-tab which renders SystemAgentsSection', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: 'System Agents' }))

    expect(await screen.findByRole('button', { name: 'Definitions' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Activity' })).toBeInTheDocument()
    // SystemAgentsSection renders its own "Add" affordance for definitions.
    expect(screen.queryByText('No system agent activity found.')).not.toBeInTheDocument()
  })

  it('clicking the Activity sub-tab renders SystemAgentRunsSection', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: 'System Agents' }))
    await user.click(screen.getByRole('button', { name: 'Activity' }))

    expect(await screen.findByText('No system agent activity found.')).toBeInTheDocument()
  })

  it('deep links directly to ?tab=system-agents&sub=activity on first mount', async () => {
    renderPage(['/?tab=system-agents&sub=activity'])

    expect(await screen.findByText('No system agent activity found.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Activity' })).toBeInTheDocument()
  })
})
