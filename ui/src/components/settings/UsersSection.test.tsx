import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { UsersSection } from './UsersSection'
import { createTestQueryClient } from '@/test/utils'
import { ApiError } from '@/api/client'
import {
  useUsers,
  useDeleteUser,
  useCreateUser,
  useUpdateUser,
  useResetUserPassword,
} from '@/hooks/useUsers'
import type { User } from '@/types/user'

vi.mock('@/hooks/useUsers')

function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: 'user-1',
    email: 'alice@example.com',
    display_name: 'Alice',
    role: 'viewer',
    status: 'active',
    must_change_password: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const mockDeleteAsync = vi.fn()
const mockCreateAsync = vi.fn()
const mockUpdateAsync = vi.fn()
const mockResetAsync = vi.fn()

function setupDefaultMocks(users: User[] = []) {
  vi.mocked(useUsers).mockReturnValue({ data: { users }, isLoading: false, error: undefined } as any)
  vi.mocked(useDeleteUser).mockReturnValue({ mutateAsync: mockDeleteAsync, isPending: false } as any)
  vi.mocked(useCreateUser).mockReturnValue({ mutateAsync: mockCreateAsync, isPending: false } as any)
  vi.mocked(useUpdateUser).mockReturnValue({ mutateAsync: mockUpdateAsync, isPending: false } as any)
  vi.mocked(useResetUserPassword).mockReturnValue({ mutateAsync: mockResetAsync, isPending: false } as any)
}

function renderSection() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <UsersSection />
    </QueryClientProvider>
  )
}

describe('UsersSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading spinner while users are loading', () => {
    vi.mocked(useUsers).mockReturnValue({ isLoading: true, data: undefined, error: undefined } as any)
    vi.mocked(useDeleteUser).mockReturnValue({ mutateAsync: mockDeleteAsync, isPending: false } as any)
    vi.mocked(useCreateUser).mockReturnValue({ mutateAsync: mockCreateAsync, isPending: false } as any)
    vi.mocked(useUpdateUser).mockReturnValue({ mutateAsync: mockUpdateAsync, isPending: false } as any)
    vi.mocked(useResetUserPassword).mockReturnValue({ mutateAsync: mockResetAsync, isPending: false } as any)

    renderSection()
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('renders user count and table with user rows', () => {
    const users = [
      makeUser({ id: 'u1', email: 'alice@example.com', display_name: 'Alice', role: 'admin' }),
      makeUser({ id: 'u2', email: 'bob@example.com', display_name: 'Bob', role: 'viewer' }),
    ]
    setupDefaultMocks(users)
    renderSection()

    expect(screen.getByText('2 users')).toBeInTheDocument()
    expect(screen.getByText('alice@example.com')).toBeInTheDocument()
    expect(screen.getByText('bob@example.com')).toBeInTheDocument()
  })

  it('shows singular user count for single user', () => {
    setupDefaultMocks([makeUser()])
    renderSection()
    expect(screen.getByText('1 user')).toBeInTheDocument()
  })

  it('shows must_change_password badge for flagged users', () => {
    setupDefaultMocks([
      makeUser({ id: 'u1', must_change_password: true }),
      makeUser({ id: 'u2', email: 'clean@example.com', must_change_password: false }),
    ])
    renderSection()

    const yesBadges = screen.getAllByText('yes')
    expect(yesBadges).toHaveLength(1)
  })

  it('hides delete button for system users', () => {
    setupDefaultMocks([
      makeUser({ id: 'sys', email: 'system@example.com', system: true }),
      makeUser({ id: 'reg', email: 'regular@example.com', system: false }),
    ])
    renderSection()

    // Only one delete button for the non-system user
    const deleteButtons = screen.getAllByTitle('Delete')
    expect(deleteButtons).toHaveLength(1)
  })

  it('clicking Create User button opens the create dialog', async () => {
    setupDefaultMocks([])
    renderSection()

    await userEvent.click(screen.getByRole('button', { name: /create user/i }))
    // Dialog is open — email input is visible
    expect(screen.getByPlaceholderText('user@example.com')).toBeInTheDocument()
  })

  it('clicking Edit button opens the edit dialog for that user', async () => {
    const user = makeUser({ email: 'alice@example.com', display_name: 'Alice' })
    setupDefaultMocks([user])
    renderSection()

    await userEvent.click(screen.getByTitle('Edit'))
    expect(screen.getByText(/Edit User.*alice@example\.com/)).toBeInTheDocument()
  })

  it('clicking Reset Password button opens the reset dialog for that user', async () => {
    const user = makeUser({ display_name: 'Alice' })
    setupDefaultMocks([user])
    renderSection()

    await userEvent.click(screen.getByTitle('Reset password'))
    expect(screen.getByText(/Reset Password.*Alice/)).toBeInTheDocument()
  })

  it('clicking delete icon opens confirm dialog', async () => {
    const user = makeUser({ id: 'u1', email: 'deleteme@example.com' })
    setupDefaultMocks([user])
    renderSection()

    await userEvent.click(screen.getByTitle('Delete'))
    expect(screen.getByText('Delete this user permanently? This action cannot be undone.')).toBeInTheDocument()
  })

  it('Cancel in confirm dialog closes it without calling mutation', async () => {
    const user = makeUser({ id: 'u1' })
    setupDefaultMocks([user])
    renderSection()

    await userEvent.click(screen.getByTitle('Delete'))
    expect(screen.getByText('Delete this user permanently? This action cannot be undone.')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByText('Delete this user permanently? This action cannot be undone.')).not.toBeInTheDocument()
    expect(mockDeleteAsync).not.toHaveBeenCalled()
  })

  it('Confirm in dialog calls delete mutation with user id', async () => {
    const user = makeUser({ id: 'target-id' })
    mockDeleteAsync.mockResolvedValue(undefined)
    setupDefaultMocks([user])
    renderSection()

    await userEvent.click(screen.getByTitle('Delete'))
    // Two buttons named "Delete": table trash (has title attr) and dialog confirm (no title attr)
    const allDeleteBtns = screen.getAllByRole('button', { name: 'Delete' })
    const dialogConfirmBtn = allDeleteBtns.find((b) => !b.getAttribute('title'))!
    await userEvent.click(dialogConfirmBtn)
    expect(mockDeleteAsync).toHaveBeenCalledWith('target-id')
  })
})
