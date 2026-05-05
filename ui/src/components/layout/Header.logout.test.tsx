import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Header } from './Header'

let mockLogout = vi.fn()
const mockNavigate = vi.fn()

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn(() => ({
    currentProject: 'test-project',
    projects: [
      {
        id: 'test-project',
        name: 'Test Project',
        root_path: '/test',
        default_branch: 'main',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    ],
    setCurrentProject: vi.fn(),
  })),
}))

vi.mock('@/stores/themeStore', () => ({
  useThemeStore: vi.fn(() => ({ theme: 'system', setTheme: vi.fn() })),
}))

vi.mock('@/stores/authStore', () => ({
  useAuthStore: vi.fn((sel: (s: { logout: typeof mockLogout }) => unknown) =>
    sel({ logout: mockLogout }),
  ),
  useIsAdmin: vi.fn(() => false),
}))

vi.mock('react-router-dom', async (importOriginal) => {
  const mod = await importOriginal<typeof import('react-router-dom')>()
  return { ...mod, useNavigate: () => mockNavigate }
})

vi.mock('./DailyStats', () => ({ DailyStats: () => <div /> }))
vi.mock('./RunningAgentsIndicator', () => ({ RunningAgentsIndicator: () => <div /> }))
vi.mock('@/components/ui/ProjectSelect', () => ({ ProjectSelect: () => <div /> }))

function renderHeader() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/']}>
        <Header />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('Header - Logout button', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockLogout = vi.fn()
    mockNavigate.mockReset()
  })

  it('renders log out button for non-admin and settings link is absent', () => {
    renderHeader()
    expect(screen.getByRole('button', { name: /log out/i })).toBeInTheDocument()
    expect(screen.queryByTitle('Settings')).toBeNull()
  })

  it('calls logout then navigates to /login with replace:true on click', async () => {
    mockLogout.mockResolvedValueOnce(undefined)
    const user = userEvent.setup()
    renderHeader()
    await user.click(screen.getByRole('button', { name: /log out/i }))
    expect(mockLogout).toHaveBeenCalledTimes(1)
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith('/login', { replace: true }),
    )
  })

  it('navigates to /login even when logout rejects', async () => {
    mockLogout.mockRejectedValueOnce(new Error('network error'))
    const user = userEvent.setup()
    renderHeader()
    await user.click(screen.getByRole('button', { name: /log out/i }))
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith('/login', { replace: true }),
    )
  })

  it('disables the button while logout is in progress', async () => {
    mockLogout = vi.fn(() => new Promise<void>(() => {}))
    const user = userEvent.setup()
    renderHeader()
    const button = screen.getByRole('button', { name: /log out/i })
    await user.click(button)
    expect(button).toBeDisabled()
  })
})
