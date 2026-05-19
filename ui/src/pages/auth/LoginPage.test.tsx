import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { LoginPage } from './LoginPage'
import { useAuthStore } from '@/stores/authStore'
import { ApiError } from '@/api/client'

let mockActiveConn = { id: 'local', name: 'Local', baseURL: '', isLocal: true }
const mockSetActive = vi.fn()

vi.mock('@/stores/connectionsStore', () => ({
  useConnectionsStore: Object.assign(
    vi.fn((selector: (s: { setActive: typeof mockSetActive }) => unknown) =>
      selector({ setActive: mockSetActive })
    ),
    { getState: vi.fn(() => ({ active: () => mockActiveConn })) }
  ),
}))

function LocationDisplay() {
  const loc = useLocation()
  return <div data-testid="location">{loc.pathname}{loc.search}</div>
}

function renderLoginPage(initialPath = '/login') {
  render(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/" element={<LocationDisplay />} />
        <Route path="/tickets" element={<LocationDisplay />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('LoginPage', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'anon' })
    vi.clearAllMocks()
  })

  it('renders email, password, and submit fields', () => {
    renderLoginPage()
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument()
  })

  it('renders heading', () => {
    renderLoginPage()
    expect(screen.getByText('Sign in to nrflo')).toBeInTheDocument()
  })

  it('navigates to / after successful login', async () => {
    const user = userEvent.setup()
    const mockLogin = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ login: mockLogin })

    renderLoginPage()

    await user.type(screen.getByLabelText(/email/i), 'user@example.com')
    await user.type(screen.getByLabelText(/password/i), 'secret')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(mockLogin).toHaveBeenCalledWith('user@example.com', 'secret')
    expect(screen.getByTestId('location')).toHaveTextContent('/')
  })

  it('navigates to ?next= path after successful login', async () => {
    const user = userEvent.setup()
    const mockLogin = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ login: mockLogin })

    renderLoginPage('/login?next=%2Ftickets')

    await user.type(screen.getByLabelText(/email/i), 'user@example.com')
    await user.type(screen.getByLabelText(/password/i), 'pw')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(screen.getByTestId('location')).toHaveTextContent('/tickets')
  })

  it('shows "Invalid credentials" on 401 error', async () => {
    const user = userEvent.setup()
    const mockLogin = vi.fn().mockRejectedValue(new ApiError(401, 'Unauthorized'))
    useAuthStore.setState({ login: mockLogin })

    renderLoginPage()

    await user.type(screen.getByLabelText(/email/i), 'user@example.com')
    await user.type(screen.getByLabelText(/password/i), 'wrong')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText('Invalid credentials')).toBeInTheDocument()
  })

  it('shows rate-limit message on 429 error', async () => {
    const user = userEvent.setup()
    const mockLogin = vi.fn().mockRejectedValue(new ApiError(429, 'too many login attempts'))
    useAuthStore.setState({ login: mockLogin })

    renderLoginPage()

    await user.type(screen.getByLabelText(/email/i), 'user@example.com')
    await user.type(screen.getByLabelText(/password/i), 'pw')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText('Too many attempts. Try again later.')).toBeInTheDocument()
  })

  it('shows error message for other ApiError', async () => {
    const user = userEvent.setup()
    const mockLogin = vi.fn().mockRejectedValue(new ApiError(500, 'Server error'))
    useAuthStore.setState({ login: mockLogin })

    renderLoginPage()

    await user.type(screen.getByLabelText(/email/i), 'u@x.com')
    await user.type(screen.getByLabelText(/password/i), 'pw')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText('Server error')).toBeInTheDocument()
  })

  it('shows generic message for non-ApiError', async () => {
    const user = userEvent.setup()
    const mockLogin = vi.fn().mockRejectedValue(new Error('network'))
    useAuthStore.setState({ login: mockLogin })

    renderLoginPage()

    await user.type(screen.getByLabelText(/email/i), 'u@x.com')
    await user.type(screen.getByLabelText(/password/i), 'pw')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText('Login failed')).toBeInTheDocument()
  })

  it('shows "Signing in…" and disables button while pending', async () => {
    const user = userEvent.setup()
    const mockLogin = vi.fn().mockReturnValue(new Promise(() => {}))
    useAuthStore.setState({ login: mockLogin })

    renderLoginPage()

    await user.type(screen.getByLabelText(/email/i), 'u@x.com')
    await user.type(screen.getByLabelText(/password/i), 'pw')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(screen.getByRole('button', { name: /signing in/i })).toBeDisabled()
  })

  it('accepts plain username (non-email) without HTML5 validation blocking submit', async () => {
    const user = userEvent.setup()
    const mockLogin = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ login: mockLogin })

    renderLoginPage()

    await user.type(screen.getByLabelText(/email or username/i), 'admin')
    await user.type(screen.getByLabelText(/password/i), 'admin')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(mockLogin).toHaveBeenCalledWith('admin', 'admin')
  })

  it('ignores non-root next param (external redirect attempt)', async () => {
    const user = userEvent.setup()
    const mockLogin = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ login: mockLogin })

    renderLoginPage('/login?next=http%3A%2F%2Fevil.com')

    await user.type(screen.getByLabelText(/email/i), 'u@x.com')
    await user.type(screen.getByLabelText(/password/i), 'pw')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(screen.getByTestId('location')).toHaveTextContent('/')
  })
})

describe('LoginPage - remote connection guard', () => {
  beforeEach(() => {
    mockSetActive.mockReset()
    mockActiveConn = { id: 'local', name: 'Local', baseURL: '', isLocal: true }
    useAuthStore.setState({ user: null, status: 'anon' })
    vi.clearAllMocks()
  })

  it('shows remote panel instead of login form when remote connection is active', () => {
    mockActiveConn = { id: 'r1', name: 'Staging Server', baseURL: 'https://staging.example.com', isLocal: false }
    renderLoginPage()
    expect(screen.queryByLabelText(/email/i)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /switch to local/i })).toBeInTheDocument()
  })

  it('shows remote connection name in the panel', () => {
    mockActiveConn = { id: 'r1', name: 'Staging Server', baseURL: 'https://staging.example.com', isLocal: false }
    renderLoginPage()
    expect(screen.getByText(/staging server/i)).toBeInTheDocument()
  })

  it('clicking Switch to Local calls setActive("local")', async () => {
    mockActiveConn = { id: 'r1', name: 'Staging Server', baseURL: 'https://staging.example.com', isLocal: false }
    const user = userEvent.setup()
    renderLoginPage()
    await user.click(screen.getByRole('button', { name: /switch to local/i }))
    expect(mockSetActive).toHaveBeenCalledWith('local')
  })

  it('shows login form (not remote panel) when local connection is active', () => {
    renderLoginPage()
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /switch to local/i })).not.toBeInTheDocument()
  })
})
