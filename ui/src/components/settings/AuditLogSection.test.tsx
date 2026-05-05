import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { AuditLogSection } from './AuditLogSection'
import { createTestQueryClient } from '@/test/utils'
import { useAuditLog } from '@/hooks/useAuditLog'
import { useUsers } from '@/hooks/useUsers'
import type { AuditEntry } from '@/types/audit'

vi.mock('@/hooks/useAuditLog')
vi.mock('@/hooks/useUsers')

function makeEntry(overrides: Partial<AuditEntry> = {}): AuditEntry {
  return {
    id: 'entry-1',
    action: 'user.login',
    resource_type: 'user',
    resource_id: 'u-1',
    ip: '127.0.0.1',
    user_agent: 'test',
    metadata: '{}',
    created_at: '2026-01-01T12:00:00Z',
    ...overrides,
  }
}

function renderSection() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <AuditLogSection />
    </QueryClientProvider>
  )
}

describe('AuditLogSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useUsers).mockReturnValue({ data: { users: [] } } as any)
  })

  it('shows loading spinner while fetching', () => {
    vi.mocked(useAuditLog).mockReturnValue({ isLoading: true, data: undefined, error: undefined } as any)
    renderSection()
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('shows empty state when no entries', () => {
    vi.mocked(useAuditLog).mockReturnValue({
      isLoading: false,
      error: undefined,
      data: { items: [], total: 0, page: 1, per_page: 50 },
    } as any)
    renderSection()
    expect(screen.getByText('No audit entries found.')).toBeInTheDocument()
  })

  it('shows total count text', () => {
    vi.mocked(useAuditLog).mockReturnValue({
      isLoading: false,
      error: undefined,
      data: { items: [], total: 42, page: 1, per_page: 50 },
    } as any)
    renderSection()
    expect(screen.getByText('42 total entries')).toBeInTheDocument()
  })

  it('renders audit entry rows with action, ip, and resource', () => {
    vi.mocked(useAuditLog).mockReturnValue({
      isLoading: false,
      error: undefined,
      data: {
        items: [makeEntry({ action: 'user.login', ip: '10.0.0.1', resource_type: 'user', resource_id: 'abc' })],
        total: 1,
        page: 1,
        per_page: 50,
      },
    } as any)
    renderSection()
    expect(screen.getByText('user.login')).toBeInTheDocument()
    expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
    expect(screen.getByText('user/abc')).toBeInTheDocument()
  })

  it('shows pagination controls when total exceeds per-page', () => {
    vi.mocked(useAuditLog).mockReturnValue({
      isLoading: false,
      error: undefined,
      data: { items: [makeEntry()], total: 101, page: 1, per_page: 50 },
    } as any)
    renderSection()
    expect(screen.getByRole('button', { name: /next/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /prev/i })).toBeInTheDocument()
  })

  it('Prev button is disabled on first page', () => {
    vi.mocked(useAuditLog).mockReturnValue({
      isLoading: false,
      error: undefined,
      data: { items: [makeEntry()], total: 101, page: 1, per_page: 50 },
    } as any)
    renderSection()
    expect(screen.getByRole('button', { name: /prev/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /next/i })).not.toBeDisabled()
  })

  it('clicking Next calls useAuditLog with page 2', async () => {
    vi.mocked(useAuditLog).mockReturnValue({
      isLoading: false,
      error: undefined,
      data: { items: [makeEntry()], total: 101, page: 1, per_page: 50 },
    } as any)
    renderSection()

    await userEvent.click(screen.getByRole('button', { name: /next/i }))

    // After clicking next, hook should be called with page=2
    const calls = vi.mocked(useAuditLog).mock.calls
    const lastCall = calls[calls.length - 1][0]
    expect(lastCall).toMatchObject({ page: 2 })
  })

  it('action filter input resets page to 1', async () => {
    vi.mocked(useAuditLog).mockReturnValue({
      isLoading: false,
      error: undefined,
      data: { items: [], total: 0, page: 1, per_page: 50 },
    } as any)
    renderSection()

    await userEvent.type(screen.getByPlaceholderText(/filter by action/i), 'login')

    const calls = vi.mocked(useAuditLog).mock.calls
    const lastCall = calls[calls.length - 1][0]
    expect(lastCall).toMatchObject({ page: 1, action: 'login' })
  })
})
