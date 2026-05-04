import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { RequireAuth } from './RequireAuth'
import { RequireAdmin } from './RequireAdmin'
import { useAuthStore } from '@/stores/authStore'
import type { User } from '@/types/user'

function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: 'u1',
    email: 'user@example.com',
    display_name: 'User',
    role: 'admin',
    status: 'active',
    must_change_password: false,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

function LocationDisplay() {
  const loc = useLocation()
  return <div data-testid="location">{loc.pathname}{loc.search}</div>
}

function renderWithAuth(
  element: React.ReactNode,
  initialPath = '/protected',
  extraRoutes?: React.ReactNode
) {
  render(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/login" element={<LocationDisplay />} />
        <Route path="/account" element={<LocationDisplay />} />
        <Route path="/forbidden" element={<LocationDisplay />} />
        {extraRoutes}
        <Route path="*" element={element} />
      </Routes>
    </MemoryRouter>
  )
}

describe('RequireAuth', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'anon' })
  })

  it('redirects anon user to /login with next param', () => {
    useAuthStore.setState({ user: null, status: 'anon' })
    renderWithAuth(<RequireAuth><LocationDisplay /></RequireAuth>, '/protected')
    expect(screen.getByTestId('location')).toHaveTextContent('/login?next=%2Fprotected')
  })

  it('encodes next param including search in redirect', () => {
    useAuthStore.setState({ user: null, status: 'anon' })
    renderWithAuth(<RequireAuth><LocationDisplay /></RequireAuth>, '/tickets?status=open')
    expect(screen.getByTestId('location')).toHaveTextContent(
      '/login?next=%2Ftickets%3Fstatus%3Dopen'
    )
  })

  it('renders children for authed user', () => {
    useAuthStore.setState({ user: makeUser(), status: 'authed' })
    renderWithAuth(<RequireAuth><LocationDisplay /></RequireAuth>, '/protected')
    expect(screen.getByTestId('location')).toHaveTextContent('/protected')
  })

  it('redirects must_change_password user to /account?force=1', () => {
    useAuthStore.setState({
      user: makeUser({ must_change_password: true }),
      status: 'authed',
    })
    renderWithAuth(<RequireAuth><LocationDisplay /></RequireAuth>, '/tickets')
    expect(screen.getByTestId('location')).toHaveTextContent('/account?force=1')
  })

  it('does NOT redirect must_change_password user when already on /account', () => {
    useAuthStore.setState({
      user: makeUser({ must_change_password: true }),
      status: 'authed',
    })
    render(
      <MemoryRouter initialEntries={['/account']}>
        <Routes>
          <Route path="/account" element={<RequireAuth><LocationDisplay /></RequireAuth>} />
        </Routes>
      </MemoryRouter>
    )
    expect(screen.getByTestId('location')).toHaveTextContent('/account')
  })

  it('renders Outlet when no children provided', () => {
    useAuthStore.setState({ user: makeUser(), status: 'authed' })
    render(
      <MemoryRouter initialEntries={['/protected']}>
        <Routes>
          <Route element={<RequireAuth />}>
            <Route path="/protected" element={<div data-testid="outlet-content">outlet</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    )
    expect(screen.getByTestId('outlet-content')).toBeInTheDocument()
  })
})

describe('RequireAdmin', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'anon' })
  })

  it('redirects anon user to /login', () => {
    useAuthStore.setState({ user: null, status: 'anon' })
    renderWithAuth(<RequireAdmin><LocationDisplay /></RequireAdmin>, '/admin')
    expect(screen.getByTestId('location')).toHaveTextContent('/login')
  })

  it('redirects viewer user to /forbidden', () => {
    useAuthStore.setState({ user: makeUser({ role: 'viewer' }), status: 'authed' })
    renderWithAuth(<RequireAdmin><LocationDisplay /></RequireAdmin>, '/admin')
    expect(screen.getByTestId('location')).toHaveTextContent('/forbidden')
  })

  it('renders children for admin user', () => {
    useAuthStore.setState({ user: makeUser({ role: 'admin' }), status: 'authed' })
    renderWithAuth(<RequireAdmin><LocationDisplay /></RequireAdmin>, '/admin')
    expect(screen.getByTestId('location')).toHaveTextContent('/admin')
  })

  it('redirects must_change_password admin to /account?force=1 before admin check', () => {
    useAuthStore.setState({
      user: makeUser({ role: 'admin', must_change_password: true }),
      status: 'authed',
    })
    renderWithAuth(<RequireAdmin><LocationDisplay /></RequireAdmin>, '/admin')
    expect(screen.getByTestId('location')).toHaveTextContent('/account?force=1')
  })
})
