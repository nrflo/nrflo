import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { AccountPage } from './AccountPage'
import { useAuthStore } from '@/stores/authStore'
import * as authApi from '@/api/auth'
import { ApiError } from '@/api/client'

vi.mock('@/api/auth')

function LocationDisplay() {
  const loc = useLocation()
  return <div data-testid="location">{loc.pathname}{loc.search}</div>
}

function renderAccountPage(initialPath = '/account') {
  render(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/account" element={<AccountPage />} />
        <Route path="/" element={<LocationDisplay />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('AccountPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.setState({ user: null, status: 'authed', refresh: vi.fn() })
  })

  it('renders change password form fields', () => {
    renderAccountPage()
    expect(screen.getByLabelText(/current password/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/new password/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/confirm password/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /change password/i })).toBeInTheDocument()
  })

  it('renders heading', () => {
    renderAccountPage()
    expect(screen.getByRole('heading', { name: 'Change Password' })).toBeInTheDocument()
  })

  it('shows force-change banner when ?force=1', () => {
    renderAccountPage('/account?force=1')
    expect(
      screen.getByText('You must change your password before continuing.')
    ).toBeInTheDocument()
  })

  it('does NOT show force-change banner without ?force=1', () => {
    renderAccountPage('/account')
    expect(
      screen.queryByText('You must change your password before continuing.')
    ).not.toBeInTheDocument()
  })

  it('shows error when new password and confirm do not match', async () => {
    const user = userEvent.setup()
    renderAccountPage()

    await user.type(screen.getByLabelText(/current password/i), 'oldpass')
    await user.type(screen.getByLabelText(/new password/i), 'newpass1')
    await user.type(screen.getByLabelText(/confirm password/i), 'newpass2')
    await user.click(screen.getByRole('button', { name: /change password/i }))

    expect(screen.getByText('Passwords do not match')).toBeInTheDocument()
    expect(authApi.changePassword).not.toHaveBeenCalled()
  })

  it('calls changePassword with current and new password on valid submit', async () => {
    const user = userEvent.setup()
    vi.mocked(authApi.changePassword).mockResolvedValue(undefined)
    const mockRefresh = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ refresh: mockRefresh })

    renderAccountPage()

    await user.type(screen.getByLabelText(/current password/i), 'oldpass')
    await user.type(screen.getByLabelText(/new password/i), 'newpass')
    await user.type(screen.getByLabelText(/confirm password/i), 'newpass')
    await user.click(screen.getByRole('button', { name: /change password/i }))

    expect(authApi.changePassword).toHaveBeenCalledWith('oldpass', 'newpass')
  })

  it('calls refresh and navigates to / on success', async () => {
    const user = userEvent.setup()
    vi.mocked(authApi.changePassword).mockResolvedValue(undefined)
    const mockRefresh = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ refresh: mockRefresh })

    renderAccountPage()

    await user.type(screen.getByLabelText(/current password/i), 'oldpass')
    await user.type(screen.getByLabelText(/new password/i), 'newpass')
    await user.type(screen.getByLabelText(/confirm password/i), 'newpass')
    await user.click(screen.getByRole('button', { name: /change password/i }))

    expect(mockRefresh).toHaveBeenCalled()
    expect(await screen.findByTestId('location')).toHaveTextContent('/')
  })

  it('shows API error message on failure', async () => {
    const user = userEvent.setup()
    vi.mocked(authApi.changePassword).mockRejectedValue(new ApiError(400, 'Wrong current password'))

    renderAccountPage()

    await user.type(screen.getByLabelText(/current password/i), 'wrong')
    await user.type(screen.getByLabelText(/new password/i), 'newpass')
    await user.type(screen.getByLabelText(/confirm password/i), 'newpass')
    await user.click(screen.getByRole('button', { name: /change password/i }))

    expect(await screen.findByText('Wrong current password')).toBeInTheDocument()
  })

  it('shows generic message on non-ApiError', async () => {
    const user = userEvent.setup()
    vi.mocked(authApi.changePassword).mockRejectedValue(new Error('network'))

    renderAccountPage()

    await user.type(screen.getByLabelText(/current password/i), 'oldpass')
    await user.type(screen.getByLabelText(/new password/i), 'newpass')
    await user.type(screen.getByLabelText(/confirm password/i), 'newpass')
    await user.click(screen.getByRole('button', { name: /change password/i }))

    expect(await screen.findByText('Failed to change password')).toBeInTheDocument()
  })

  it('shows "Changing…" and disables button while pending', async () => {
    const user = userEvent.setup()
    vi.mocked(authApi.changePassword).mockReturnValue(new Promise(() => {}))

    renderAccountPage()

    await user.type(screen.getByLabelText(/current password/i), 'oldpass')
    await user.type(screen.getByLabelText(/new password/i), 'newpass')
    await user.type(screen.getByLabelText(/confirm password/i), 'newpass')
    await user.click(screen.getByRole('button', { name: /change password/i }))

    expect(screen.getByRole('button', { name: /changing/i })).toBeDisabled()
  })
})
