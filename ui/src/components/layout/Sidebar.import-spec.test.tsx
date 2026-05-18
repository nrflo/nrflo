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
vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => mockUseAPIModeEnabled(),
  useExperimentalEnabled: () => mockUseExperimentalEnabled(),
}))

const mockUseIsAdmin = vi.fn().mockReturnValue(false)
vi.mock('@/stores/authStore', () => ({
  useIsAdmin: () => mockUseIsAdmin(),
}))

const mockUseChainList = vi.fn()
vi.mock('@/hooks/useChains', () => ({
  useChainList: () => mockUseChainList(),
}))

function renderSidebar(initialRoute = '/') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialRoute]}>
        <Sidebar />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('Sidebar - Import Spec nav item', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseStatus.mockReturnValue({ data: undefined })
    mockUseRunningAgents.mockReturnValue({ data: { agents: [], count: 0 } })
    mockUseChainList.mockReturnValue({ data: [] })
    useProjectStore.setState({ currentProject: 'proj-test', projects: [] })
  })

  it('renders Import Spec link', () => {
    renderSidebar()
    expect(screen.getByRole('link', { name: /import spec/i })).toBeInTheDocument()
  })

  it('links to /import', () => {
    renderSidebar()
    expect(screen.getByRole('link', { name: /import spec/i })).toHaveAttribute('href', '/import')
  })

  it('shows active styling when on /import', () => {
    renderSidebar('/import')
    expect(screen.getByRole('link', { name: /import spec/i })).toHaveClass('bg-muted', 'text-foreground')
  })

  it('shows active styling for nested /import/* paths', () => {
    renderSidebar('/import/something')
    expect(screen.getByRole('link', { name: /import spec/i })).toHaveClass('bg-muted', 'text-foreground')
  })

  it('does not show active styling when on a different route', () => {
    renderSidebar('/tickets')
    const link = screen.getByRole('link', { name: /import spec/i })
    expect(link).not.toHaveClass('bg-muted', 'text-foreground')
    expect(link).toHaveClass('text-muted-foreground')
  })

  it('appears after New Ticket and before Project Workflows', () => {
    renderSidebar()
    const allLinks = screen.getAllByRole('link')
    const newTicketIdx = allLinks.findIndex((l) => l.textContent?.includes('New Ticket'))
    const importSpecIdx = allLinks.findIndex((l) => l.textContent?.includes('Import Spec'))
    const projectWorkflowsIdx = allLinks.findIndex((l) => l.textContent?.includes('Project Workflows'))

    expect(newTicketIdx).toBeGreaterThan(-1)
    expect(importSpecIdx).toBeGreaterThan(-1)
    expect(projectWorkflowsIdx).toBeGreaterThan(-1)
    expect(importSpecIdx).toBeGreaterThan(newTicketIdx)
    expect(importSpecIdx).toBeLessThan(projectWorkflowsIdx)
  })
})
