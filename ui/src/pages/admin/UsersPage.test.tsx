import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { UsersPage } from './UsersPage'
import type { User } from '@/types/user'
import { ApiError } from '@/api/client'
// ApiError used for testing create form error paths

const mockUseUsers = vi.fn()
const mockCreateMutateAsync = vi.fn()
const mockDeleteMutateAsync = vi.fn()

vi.mock('@/hooks/useUsers', () => ({
  useUsers: () => mockUseUsers(),
  useCreateUser: () => ({ mutateAsync: mockCreateMutateAsync, isPending: false }),
  useUpdateUser: () => ({ mutateAsync: vi.fn().mockResolvedValue(undefined), isPending: false }),
  useResetUserPassword: () => ({ mutateAsync: vi.fn().mockResolvedValue(undefined), isPending: false }),
  useDeleteUser: () => ({ mutateAsync: mockDeleteMutateAsync, isPending: false }),
}))

function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: 'user-1',
    email: 'alice@example.com',
    display_name: 'Alice',
    role: 'admin',
    status: 'active',
    must_change_password: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function setupLoaded(users: User[] = []) {
  mockUseUsers.mockReturnValue({ data: { users }, isLoading: false, error: null })
}

beforeEach(() => {
  vi.clearAllMocks()
  mockCreateMutateAsync.mockResolvedValue(undefined)
  mockDeleteMutateAsync.mockResolvedValue(undefined)
  mockUseUsers.mockReturnValue({ data: { users: [] }, isLoading: false, error: null })
})

describe('UsersPage - render states', () => {
  it('shows loading spinner while loading', () => {
    mockUseUsers.mockReturnValue({ data: undefined, isLoading: true, error: null })
    render(<UsersPage />)
    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument()
  })

  it('shows error message on query failure', () => {
    mockUseUsers.mockReturnValue({ data: undefined, isLoading: false, error: new Error('Server error') })
    render(<UsersPage />)
    expect(screen.getByText('Server error')).toBeInTheDocument()
  })

  it('renders user rows with email, role, status', () => {
    setupLoaded([
      makeUser({ email: 'alice@example.com', role: 'admin', status: 'active' }),
      makeUser({ id: 'user-2', email: 'bob@example.com', role: 'viewer', status: 'disabled' }),
    ])
    render(<UsersPage />)
    expect(screen.getByText('alice@example.com')).toBeInTheDocument()
    expect(screen.getByText('bob@example.com')).toBeInTheDocument()
    expect(screen.getByText('admin')).toBeInTheDocument()
    expect(screen.getByText('viewer')).toBeInTheDocument()
    expect(screen.getByText('active')).toBeInTheDocument()
    expect(screen.getByText('disabled')).toBeInTheDocument()
  })

  it('shows correct user count', () => {
    setupLoaded([makeUser(), makeUser({ id: 'user-2', email: 'b@b.com' })])
    render(<UsersPage />)
    expect(screen.getByText('2 users')).toBeInTheDocument()
  })

  it('shows singular count for one user', () => {
    setupLoaded([makeUser()])
    render(<UsersPage />)
    expect(screen.getByText('1 user')).toBeInTheDocument()
  })

  it('shows must-change-password badge when set', () => {
    setupLoaded([makeUser({ must_change_password: true })])
    render(<UsersPage />)
    expect(screen.getByText('yes')).toBeInTheDocument()
  })

  it('shows dash for last_login_at when absent', () => {
    setupLoaded([makeUser({ last_login_at: undefined })])
    render(<UsersPage />)
    expect(screen.getAllByText('—').length).toBeGreaterThan(0)
  })
})

describe('UsersPage - create flow', () => {
  it('opens CreateUserDialog when Create User is clicked', async () => {
    const user = userEvent.setup()
    setupLoaded([])
    render(<UsersPage />)

    await user.click(screen.getByRole('button', { name: /Create User/i }))

    expect(screen.getByPlaceholderText('user@example.com')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Jane Doe')).toBeInTheDocument()
  })

  it('submitting create form calls createUser mutation with correct payload', async () => {
    const user = userEvent.setup()
    setupLoaded([])
    render(<UsersPage />)

    const pageCreateBtn = screen.getByRole('button', { name: /Create User/i })
    await user.click(pageCreateBtn)

    await user.type(screen.getByPlaceholderText('user@example.com'), 'new@example.com')
    await user.type(screen.getByPlaceholderText('Jane Doe'), 'New User')
    await user.type(screen.getByPlaceholderText('8-128 characters'), 'password123')

    // Dialog's submit button is the last "Create User" button in the DOM
    const allCreateBtns = screen.getAllByRole('button', { name: /Create User/i })
    await user.click(allCreateBtns[allCreateBtns.length - 1])

    expect(mockCreateMutateAsync).toHaveBeenCalledWith({
      email: 'new@example.com',
      display_name: 'New User',
      password: 'password123',
      role: 'viewer',
    })
  })

  it('shows email_exists error message when create rejects with that code', async () => {
    const user = userEvent.setup()
    mockCreateMutateAsync.mockRejectedValueOnce(new ApiError(409, 'email_exists'))
    setupLoaded([])
    render(<UsersPage />)

    await user.click(screen.getByRole('button', { name: /Create User/i }))
    await user.type(screen.getByPlaceholderText('user@example.com'), 'dup@example.com')
    await user.type(screen.getByPlaceholderText('Jane Doe'), 'Dup User')
    await user.type(screen.getByPlaceholderText('8-128 characters'), 'password123')

    const allCreateBtns = screen.getAllByRole('button', { name: /Create User/i })
    await user.click(allCreateBtns[allCreateBtns.length - 1])

    expect(await screen.findByText('A user with this email already exists.')).toBeInTheDocument()
  })
})

describe('UsersPage - delete flow', () => {
  it('opens confirm dialog when Delete trash button is clicked', async () => {
    const user = userEvent.setup()
    setupLoaded([makeUser()])
    render(<UsersPage />)

    await user.click(screen.getByTitle('Delete'))

    expect(screen.getByText('Delete User')).toBeInTheDocument()
    expect(screen.getByText(/Delete this user permanently/)).toBeInTheDocument()
  })

  it('calls deleteUser mutation on confirm', async () => {
    const user = userEvent.setup()
    setupLoaded([makeUser({ id: 'user-abc' })])
    render(<UsersPage />)

    await user.click(screen.getByTitle('Delete'))
    await user.click(screen.getByText('Delete', { selector: 'button' }))

    expect(mockDeleteMutateAsync).toHaveBeenCalledWith('user-abc')
  })

  it('cancelling delete confirm closes dialog', async () => {
    const user = userEvent.setup()
    setupLoaded([makeUser()])
    render(<UsersPage />)

    await user.click(screen.getByTitle('Delete'))
    expect(screen.getByText('Delete User')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /cancel/i }))
    expect(screen.queryByText('Delete User')).not.toBeInTheDocument()
  })
})

describe('UsersPage - action buttons', () => {
  it('clicking Edit opens EditUserDialog', async () => {
    const user = userEvent.setup()
    setupLoaded([makeUser({ email: 'alice@example.com' })])
    render(<UsersPage />)

    await user.click(screen.getByTitle('Edit'))

    expect(screen.getByText(/Edit User — alice@example.com/)).toBeInTheDocument()
  })

  it('clicking Reset password opens ResetPasswordDialog', async () => {
    const user = userEvent.setup()
    setupLoaded([makeUser({ display_name: 'Alice' })])
    render(<UsersPage />)

    await user.click(screen.getByTitle('Reset password'))

    expect(screen.getByText(/Reset Password — Alice/)).toBeInTheDocument()
  })
})

describe('UsersPage - system user', () => {
  it('hides Delete button for system users', () => {
    setupLoaded([makeUser({ id: 'user-sys', system: true })])
    render(<UsersPage />)
    expect(screen.queryByTitle('Delete')).not.toBeInTheDocument()
  })

  it('shows Edit and Reset Password buttons for system users', () => {
    setupLoaded([makeUser({ id: 'user-sys', system: true })])
    render(<UsersPage />)
    expect(screen.getByTitle('Edit')).toBeInTheDocument()
    expect(screen.getByTitle('Reset password')).toBeInTheDocument()
  })

  it('shows Delete button for non-system users', () => {
    setupLoaded([makeUser({ system: false })])
    render(<UsersPage />)
    expect(screen.getByTitle('Delete')).toBeInTheDocument()
  })

  it('shows Delete button when system is unset', () => {
    setupLoaded([makeUser()])
    render(<UsersPage />)
    expect(screen.getByTitle('Delete')).toBeInTheDocument()
  })

  it('renders System badge for system users', () => {
    setupLoaded([makeUser({ system: true })])
    render(<UsersPage />)
    expect(screen.getByText('System')).toBeInTheDocument()
  })

  it('does not render System badge for non-system users', () => {
    setupLoaded([makeUser({ system: false })])
    render(<UsersPage />)
    expect(screen.queryByText('System')).not.toBeInTheDocument()
  })

})
