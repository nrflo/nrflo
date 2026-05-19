import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Sidebar } from './Sidebar'
import { useProjectStore } from '@/stores/projectStore'

const mockUseStatus = vi.fn()
vi.mock('@/hooks/useTickets', () => ({
  useStatus: () => mockUseStatus(),
}))

const mockUseRunningAgents = vi.fn()
vi.mock('@/hooks/useRunningAgents', () => ({
  useRunningAgents: () => mockUseRunningAgents(),
}))

const mockUseAPIModeEnabled = vi.fn().mockReturnValue(false)
const mockUseExperimentalEnabled = vi.fn().mockReturnValue(false)
const mockUseMenuVisibility = vi.fn()
vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => mockUseAPIModeEnabled(),
  useExperimentalEnabled: () => mockUseExperimentalEnabled(),
  useExperimentalObserverEnabled: () => false,
  useMenuVisibility: () => mockUseMenuVisibility(),
}))

const mockUseIsAdmin = vi.fn().mockReturnValue(true)
vi.mock('@/stores/authStore', () => ({
  useIsAdmin: () => mockUseIsAdmin(),
}))

const mockUseChainList = vi.fn()
vi.mock('@/hooks/useChains', () => ({
  useChainList: () => mockUseChainList(),
}))

const allVisible = {
  newTicket: true, importSpec: true, git: true, chainExecutions: true,
  schedules: true, workflowChains: true, pythonScripts: true,
  documentation: true, errors: true, agentSessions: true,
}

function renderSidebar() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <Sidebar />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('Sidebar - Menu Visibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseIsAdmin.mockReturnValue(true)
    mockUseAPIModeEnabled.mockReturnValue(false)
    mockUseExperimentalEnabled.mockReturnValue(false)
    mockUseMenuVisibility.mockReturnValue(allVisible)
    mockUseStatus.mockReturnValue({ data: undefined })
    mockUseRunningAgents.mockReturnValue({ data: { agents: [], count: 0 } })
    mockUseChainList.mockReturnValue({ data: [] })
    // projects=[] → hasDefaultBranch=false (no project record found)
    useProjectStore.setState({ currentProject: 'proj-test', projects: [] })
  })

  it('with all menu flags true, affected items render', () => {
    renderSidebar()
    expect(screen.getByRole('link', { name: /documentation/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /errors/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /agent sessions/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /new ticket/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /import spec/i })).toBeInTheDocument()
  })

  it('menu.documentation=false hides Documentation link', () => {
    mockUseMenuVisibility.mockReturnValue({ ...allVisible, documentation: false })
    renderSidebar()
    expect(screen.queryByRole('link', { name: /documentation/i })).not.toBeInTheDocument()
  })

  it('menu.errors=false hides Errors link', () => {
    mockUseMenuVisibility.mockReturnValue({ ...allVisible, errors: false })
    renderSidebar()
    expect(screen.queryByRole('link', { name: /errors/i })).not.toBeInTheDocument()
  })

  it('menu.agentSessions=false hides Agent Sessions link', () => {
    mockUseMenuVisibility.mockReturnValue({ ...allVisible, agentSessions: false })
    renderSidebar()
    expect(screen.queryByRole('link', { name: /agent sessions/i })).not.toBeInTheDocument()
  })

  it('menu.newTicket=false hides New Ticket link', () => {
    mockUseMenuVisibility.mockReturnValue({ ...allVisible, newTicket: false })
    renderSidebar()
    expect(screen.queryByRole('link', { name: /new ticket/i })).not.toBeInTheDocument()
  })

  it('menu.importSpec=false hides Import Spec link', () => {
    mockUseMenuVisibility.mockReturnValue({ ...allVisible, importSpec: false })
    renderSidebar()
    expect(screen.queryByRole('link', { name: /import spec/i })).not.toBeInTheDocument()
  })

  it('menu.git=true but hasDefaultBranch=false still hides Git (existing gate preserved)', () => {
    // projects=[] so hasDefaultBranch=false; menu.git=true should not override the existing gate
    mockUseMenuVisibility.mockReturnValue({ ...allVisible, git: true })
    renderSidebar()
    expect(screen.queryByRole('link', { name: /^git$/i })).not.toBeInTheDocument()
  })

  it('menu.schedules=true but isAdmin=false still hides Schedules (existing gate preserved)', () => {
    mockUseMenuVisibility.mockReturnValue({ ...allVisible, schedules: true })
    mockUseIsAdmin.mockReturnValue(false)
    renderSidebar()
    expect(screen.queryByRole('link', { name: /schedules/i })).not.toBeInTheDocument()
  })
})
