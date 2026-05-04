import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { MustChangePasswordBanner } from './MustChangePasswordBanner'
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

describe('MustChangePasswordBanner', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'anon' })
  })

  it('renders nothing when must_change_password is false', () => {
    useAuthStore.setState({ user: makeUser({ must_change_password: false }), status: 'authed' })
    const { container } = render(
      <MemoryRouter>
        <MustChangePasswordBanner />
      </MemoryRouter>
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when user is null', () => {
    useAuthStore.setState({ user: null, status: 'anon' })
    const { container } = render(
      <MemoryRouter>
        <MustChangePasswordBanner />
      </MemoryRouter>
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders banner when must_change_password is true', () => {
    useAuthStore.setState({ user: makeUser({ must_change_password: true }), status: 'authed' })
    render(
      <MemoryRouter>
        <MustChangePasswordBanner />
      </MemoryRouter>
    )
    expect(
      screen.getByText('Your password must be changed before continuing.')
    ).toBeInTheDocument()
  })

  it('renders link to /account?force=1 in banner', () => {
    useAuthStore.setState({ user: makeUser({ must_change_password: true }), status: 'authed' })
    render(
      <MemoryRouter>
        <MustChangePasswordBanner />
      </MemoryRouter>
    )
    const link = screen.getByRole('link', { name: /change it now/i })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/account?force=1')
  })
})
