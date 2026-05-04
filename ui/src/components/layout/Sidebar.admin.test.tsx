import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Sidebar } from './Sidebar'

// Replicate mock setup from Sidebar.test.tsx (each file has isolated module scope)
const mockUseStatus = vi.fn()
const mockUseProjectWorkflow = vi.fn()
vi.mock('@/hooks/useTickets', () => ({
  useStatus: () => mockUseStatus(),
  useProjectWorkflow: () => mockUseProjectWorkflow(),
}))

const mockUseAPIModeEnabled = vi.fn().mockReturnValue(false)
vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => mockUseAPIModeEnabled(),
}))

const mockUseIsAdmin = vi.fn().mockReturnValue(true)
vi.mock('@/stores/authStore', () => ({
  useIsAdmin: () => mockUseIsAdmin(),
}))

const mockUseChainList = vi.fn()
vi.mock('@/hooks/useChains', () => ({
  useChainList: () => mockUseChainList(),
}))

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

describe('Sidebar - Admin Role', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseIsAdmin.mockReturnValue(true)
    mockUseAPIModeEnabled.mockReturnValue(false)
    mockUseStatus.mockReturnValue({ data: undefined })
    mockUseProjectWorkflow.mockReturnValue({ data: undefined })
    mockUseChainList.mockReturnValue({ data: [] })
  })

  it('shows Schedules nav item', () => {
    renderSidebar()
    expect(screen.getByRole('link', { name: 'Schedules' })).toBeInTheDocument()
  })

  it('shows Administration section heading', () => {
    renderSidebar()
    expect(screen.getByText('Administration')).toBeInTheDocument()
  })

  it('shows Users nav item linking to /admin/users', () => {
    renderSidebar()
    const usersLink = screen.getByRole('link', { name: 'Users' })
    expect(usersLink).toBeInTheDocument()
    expect(usersLink).toHaveAttribute('href', '/admin/users')
  })

  it('shows Audit Log nav item linking to /admin/audit', () => {
    renderSidebar()
    const auditLink = screen.getByRole('link', { name: 'Audit Log' })
    expect(auditLink).toBeInTheDocument()
    expect(auditLink).toHaveAttribute('href', '/admin/audit')
  })

  it('shows Administration section even when API mode is disabled', () => {
    mockUseAPIModeEnabled.mockReturnValue(false)
    renderSidebar()
    expect(screen.getByText('Administration')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Users' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Audit Log' })).toBeInTheDocument()
  })
})

describe('Sidebar - Viewer Role', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseIsAdmin.mockReturnValue(false)
    mockUseAPIModeEnabled.mockReturnValue(true)
    mockUseStatus.mockReturnValue({ data: undefined })
    mockUseProjectWorkflow.mockReturnValue({ data: undefined })
    mockUseChainList.mockReturnValue({ data: [] })
  })

  it('hides Schedules nav item', () => {
    renderSidebar()
    expect(screen.queryByRole('link', { name: 'Schedules' })).not.toBeInTheDocument()
  })

  it('hides Administration section heading', () => {
    renderSidebar()
    expect(screen.queryByText('Administration')).not.toBeInTheDocument()
  })

  it('hides Users nav item', () => {
    renderSidebar()
    expect(screen.queryByRole('link', { name: 'Users' })).not.toBeInTheDocument()
  })

  it('hides Audit Log nav item', () => {
    renderSidebar()
    expect(screen.queryByRole('link', { name: 'Audit Log' })).not.toBeInTheDocument()
  })

  it('hides Configuration section and Tool Definitions even with apiModeEnabled=true', () => {
    renderSidebar()
    expect(screen.queryByText('Configuration')).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Tool Definitions' })).not.toBeInTheDocument()
  })

  it('still shows non-gated nav items', () => {
    renderSidebar()
    expect(screen.getByRole('link', { name: 'Dashboard' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Documentation' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Errors' })).toBeInTheDocument()
  })
})
