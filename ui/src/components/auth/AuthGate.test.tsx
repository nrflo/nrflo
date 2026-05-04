import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AuthGate } from './AuthGate'
import { useAuthStore } from '@/stores/authStore'

vi.mock('@/api/client', () => ({
  set401Handler: vi.fn(),
}))

import { set401Handler } from '@/api/client'

describe('AuthGate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.setState({ user: null, status: 'loading' })
  })

  it('renders null while status=loading', () => {
    useAuthStore.setState({
      status: 'loading',
      refresh: () => new Promise(() => {}),
    })
    const { container } = render(
      <AuthGate>
        <div data-testid="children">App</div>
      </AuthGate>
    )
    expect(screen.queryByTestId('children')).not.toBeInTheDocument()
    expect(container.firstChild).toBeNull()
  })

  it('renders children when status=authed', () => {
    useAuthStore.setState({ status: 'authed', refresh: vi.fn() })
    render(
      <AuthGate>
        <div data-testid="children">App</div>
      </AuthGate>
    )
    expect(screen.getByTestId('children')).toBeInTheDocument()
  })

  it('renders children when status=anon', () => {
    useAuthStore.setState({ status: 'anon', refresh: vi.fn() })
    render(
      <AuthGate>
        <div data-testid="children">App</div>
      </AuthGate>
    )
    expect(screen.getByTestId('children')).toBeInTheDocument()
  })

  it('calls set401Handler on mount', () => {
    useAuthStore.setState({ status: 'anon', refresh: vi.fn() })
    render(
      <AuthGate>
        <div>App</div>
      </AuthGate>
    )
    expect(set401Handler).toHaveBeenCalledOnce()
    expect(set401Handler).toHaveBeenCalledWith(expect.any(Function))
  })

  it('calls refresh() on mount', () => {
    const mockRefresh = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ status: 'anon', refresh: mockRefresh })
    render(
      <AuthGate>
        <div>App</div>
      </AuthGate>
    )
    expect(mockRefresh).toHaveBeenCalledOnce()
  })

  it('registered 401 handler calls clear() when not on /login', () => {
    window.location = {
      protocol: 'http:',
      host: 'localhost:5175',
      pathname: '/dashboard',
      search: '',
    } as Location
    vi.spyOn(window.history, 'pushState').mockImplementation(() => {})
    const mockClear = vi.fn()
    useAuthStore.setState({ status: 'anon', refresh: vi.fn(), clear: mockClear })

    render(
      <AuthGate>
        <div>App</div>
      </AuthGate>
    )

    const registeredHandler = vi.mocked(set401Handler).mock.calls[0][0]
    registeredHandler('/dashboard')

    expect(mockClear).toHaveBeenCalled()
    vi.restoreAllMocks()
  })

  it('registered 401 handler navigates to /login?next=... when not on /login', () => {
    window.location = {
      protocol: 'http:',
      host: 'localhost:5175',
      pathname: '/tickets',
      search: '',
    } as Location
    const pushStateSpy = vi.spyOn(window.history, 'pushState').mockImplementation(() => {})
    useAuthStore.setState({ status: 'anon', refresh: vi.fn(), clear: vi.fn() })

    render(
      <AuthGate>
        <div>App</div>
      </AuthGate>
    )

    const registeredHandler = vi.mocked(set401Handler).mock.calls[0][0]
    registeredHandler('/tickets')

    expect(pushStateSpy).toHaveBeenCalledWith({}, '', '/login?next=%2Ftickets')
    vi.restoreAllMocks()
  })

  it('registered 401 handler does NOT navigate when already on /login', () => {
    window.location = {
      protocol: 'http:',
      host: 'localhost:5175',
      pathname: '/login',
      search: '',
    } as Location
    const pushStateSpy = vi.spyOn(window.history, 'pushState').mockImplementation(() => {})
    useAuthStore.setState({ status: 'anon', refresh: vi.fn(), clear: vi.fn() })

    render(
      <AuthGate>
        <div>App</div>
      </AuthGate>
    )

    const registeredHandler = vi.mocked(set401Handler).mock.calls[0][0]
    registeredHandler('/login')

    expect(pushStateSpy).not.toHaveBeenCalled()
    vi.restoreAllMocks()
  })
})
