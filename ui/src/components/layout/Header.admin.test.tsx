import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Header } from './Header'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn(() => ({
    currentProject: 'proj1',
    projects: [{ id: 'proj1', name: 'Test Project', root_path: '/t', default_branch: null, created_at: '', updated_at: '' }],
    setCurrentProject: vi.fn(),
  })),
}))

vi.mock('@/stores/themeStore', () => ({
  useThemeStore: vi.fn(() => ({ theme: 'system', setTheme: vi.fn() })),
}))

vi.mock('@/stores/connectionsStore', () => ({
  useConnectionsStore: vi.fn(() => ({ list: [], activeId: 'local', setActive: vi.fn() })),
}))

const mockUseIsAdmin = vi.fn()
vi.mock('@/stores/authStore', () => ({
  useAuthStore: vi.fn((selector: (s: { logout: () => Promise<void> }) => unknown) =>
    selector({ logout: vi.fn().mockResolvedValue(undefined) })
  ),
  useIsAdmin: () => mockUseIsAdmin(),
}))

vi.mock('./DailyStats', () => ({ DailyStats: () => null }))
vi.mock('./RunningAgentsIndicator', () => ({ RunningAgentsIndicator: () => null }))
vi.mock('@/components/ui/ProjectSelect', () => ({ ProjectSelect: () => null }))
vi.mock('@/components/interactive/InteractiveSessionsTab', () => ({ InteractiveSessionsTab: () => null }))

function renderHeader() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <Header />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('Header — Project Settings link admin gating', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows Project Settings link when user is admin', () => {
    mockUseIsAdmin.mockReturnValue(true)
    renderHeader()
    const link = screen.getByTitle('Project Settings').closest('a')
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/project-settings')
  })

  it('hides Project Settings link when user is not admin', () => {
    mockUseIsAdmin.mockReturnValue(false)
    renderHeader()
    expect(screen.queryByTitle('Project Settings')).not.toBeInTheDocument()
  })
})
