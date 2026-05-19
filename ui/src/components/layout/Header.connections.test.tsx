import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Header } from './Header'
import type { Connection } from '@/stores/connectionsStore'

const LOCAL: Connection = { id: 'local', name: 'Local', baseURL: '', isLocal: true }
const REMOTE: Connection = { id: 'r1', name: 'Production', baseURL: 'https://prod.example.com', isLocal: false }

let mockConnList: Connection[] = [LOCAL]
let mockActiveConnId = 'local'
const mockSetActiveConn = vi.fn()

vi.mock('@/stores/connectionsStore', () => ({
  useConnectionsStore: vi.fn(() => ({
    list: mockConnList,
    activeId: mockActiveConnId,
    setActive: mockSetActiveConn,
  })),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn(() => ({
    currentProject: 'proj-1',
    projects: [{ id: 'proj-1', name: 'Test Project', root_path: '/t', default_branch: null, created_at: '', updated_at: '' }],
    setCurrentProject: vi.fn(),
  })),
}))

vi.mock('@/stores/themeStore', () => ({
  useThemeStore: vi.fn(() => ({ theme: 'system', setTheme: vi.fn() })),
}))

vi.mock('@/stores/authStore', () => ({
  useAuthStore: vi.fn((selector: (s: { logout: () => Promise<void> }) => unknown) =>
    selector({ logout: vi.fn().mockResolvedValue(undefined) })
  ),
  useIsAdmin: vi.fn().mockReturnValue(false),
}))

vi.mock('./DailyStats', () => ({ DailyStats: () => null }))
vi.mock('./RunningAgentsIndicator', () => ({ RunningAgentsIndicator: () => null }))
vi.mock('@/components/ui/ProjectSelect', () => ({
  ProjectSelect: () => <div data-testid="project-select" />,
}))
vi.mock('@/components/interactive/InteractiveSessionsTab', () => ({
  InteractiveSessionsTab: () => null,
}))

function LocationDisplay() {
  const loc = useLocation()
  return <div data-testid="location">{loc.pathname}</div>
}

function renderHeader(initialRoute = '/') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[initialRoute]}>
        <Routes>
          <Route path="*" element={<><Header /><LocationDisplay /></>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('Header - connection dropdown', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockConnList = [LOCAL]
    mockActiveConnId = 'local'
  })

  it('trigger shows the active connection name', () => {
    renderHeader()
    expect(screen.getByRole('button', { name: /local/i })).toBeInTheDocument()
  })

  it('shows remote connection name when remote is active', () => {
    mockConnList = [LOCAL, REMOTE]
    mockActiveConnId = 'r1'
    renderHeader()
    expect(screen.getByRole('button', { name: /production/i })).toBeInTheDocument()
  })

  it('selecting another connection calls setActive with its id', async () => {
    mockConnList = [LOCAL, REMOTE]
    mockActiveConnId = 'local'
    const user = userEvent.setup()
    renderHeader()
    await user.click(screen.getByRole('button', { name: /local/i }))
    await user.click(await screen.findByText('Production'))
    expect(mockSetActiveConn).toHaveBeenCalledWith('r1')
  })

  it('selecting Manage connections navigates to /settings/connections', async () => {
    const user = userEvent.setup()
    renderHeader()
    await user.click(screen.getByRole('button', { name: /local/i }))
    await user.click(await screen.findByText(/manage connections/i))
    expect(screen.getByTestId('location')).toHaveTextContent('/settings/connections')
  })

  it('dropdown lists all connections plus Manage option', async () => {
    mockConnList = [LOCAL, REMOTE]
    mockActiveConnId = 'local'
    const user = userEvent.setup()
    renderHeader()
    // Manage connections only appears once the panel opens
    expect(screen.queryByText(/manage connections/i)).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /local/i }))
    expect(await screen.findByText(/manage connections/i)).toBeInTheDocument()
    expect(screen.getByText('Production')).toBeInTheDocument()
  })
})
