import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { AuditLogPage } from './AuditLogPage'
import type { AuditEntry } from '@/types/audit'

vi.mock('@/hooks/useAuditLog', () => ({
  useAuditLog: vi.fn(),
}))

vi.mock('@/hooks/useUsers', () => ({
  useUsers: vi.fn(),
}))

import { useAuditLog } from '@/hooks/useAuditLog'
import { useUsers } from '@/hooks/useUsers'

function makeEntry(overrides: Partial<AuditEntry> = {}): AuditEntry {
  return {
    id: 'entry-1',
    user_id: 'user-1',
    action: 'user_create',
    resource_type: 'user',
    resource_id: 'usr-abc',
    ip: '127.0.0.1',
    user_agent: '',
    metadata: {},
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeListResponse(entries: AuditEntry[], total = entries.length) {
  return { items: entries, total, page: 1, per_page: 50 }
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <AuditLogPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('AuditLogPage - render states', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useUsers).mockReturnValue({ data: { users: [] }, isLoading: false, error: null } as ReturnType<typeof useUsers>)
  })

  it('shows loading spinner while loading', () => {
    vi.mocked(useAuditLog).mockReturnValue({ data: undefined, isLoading: true, error: null } as ReturnType<typeof useAuditLog>)
    renderPage()
    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument()
  })

  it('shows empty state when no entries', () => {
    vi.mocked(useAuditLog).mockReturnValue({ data: makeListResponse([]), isLoading: false, error: null } as ReturnType<typeof useAuditLog>)
    renderPage()
    expect(screen.getByText('No audit entries found.')).toBeInTheDocument()
  })

  it('renders rows with action code and ip', () => {
    vi.mocked(useAuditLog).mockReturnValue({
      data: makeListResponse([
        makeEntry({ action: 'user_create', ip: '192.168.1.1' }),
        makeEntry({ id: 'e2', action: 'password_reset_by_admin', ip: '10.0.0.5' }),
      ]),
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAuditLog>)
    renderPage()
    expect(screen.getByText('user_create')).toBeInTheDocument()
    expect(screen.getByText('password_reset_by_admin')).toBeInTheDocument()
    expect(screen.getByText('192.168.1.1')).toBeInTheDocument()
    expect(screen.getByText('10.0.0.5')).toBeInTheDocument()
  })

  it('shows total entry count', () => {
    vi.mocked(useAuditLog).mockReturnValue({ data: makeListResponse([], 99), isLoading: false, error: null } as ReturnType<typeof useAuditLog>)
    renderPage()
    expect(screen.getByText('99 total entries')).toBeInTheDocument()
  })

  it('shows resource type and id concatenated', () => {
    vi.mocked(useAuditLog).mockReturnValue({
      data: makeListResponse([makeEntry({ resource_type: 'user', resource_id: 'usr-xyz' })]),
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAuditLog>)
    renderPage()
    expect(screen.getByText('user/usr-xyz')).toBeInTheDocument()
  })

  it('resolves user display name from users list', () => {
    vi.mocked(useUsers).mockReturnValue({
      data: { users: [{ id: 'user-1', email: 'a@a.com', display_name: 'Alice', role: 'admin', status: 'active', must_change_password: false, created_at: '', updated_at: '' }] },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useUsers>)
    vi.mocked(useAuditLog).mockReturnValue({
      data: makeListResponse([makeEntry({ user_id: 'user-1' })]),
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAuditLog>)
    renderPage()
    expect(screen.getByText('Alice')).toBeInTheDocument()
  })
})

describe('AuditLogPage - pagination', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useUsers).mockReturnValue({ data: { users: [] }, isLoading: false, error: null } as ReturnType<typeof useUsers>)
    vi.mocked(useAuditLog).mockReturnValue({
      data: { items: [makeEntry()], total: 110, page: 1, per_page: 50 },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAuditLog>)
  })

  it('shows pagination controls when total exceeds per_page', () => {
    renderPage()
    expect(screen.getByRole('button', { name: /Next/i })).toBeInTheDocument()
    expect(screen.getByText(/Page 1 of 3/)).toBeInTheDocument()
  })

  it('Prev button is disabled on page 1', () => {
    renderPage()
    expect(screen.getByRole('button', { name: /Prev/i })).toBeDisabled()
  })

  it('clicking Next re-queries useAuditLog with page=2', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: /Next/i }))

    expect(vi.mocked(useAuditLog)).toHaveBeenCalledWith(
      expect.objectContaining({ page: 2, per_page: 50 })
    )
  })
})

describe('AuditLogPage - filters', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useUsers).mockReturnValue({ data: { users: [] }, isLoading: false, error: null } as ReturnType<typeof useUsers>)
    vi.mocked(useAuditLog).mockReturnValue({ data: makeListResponse([]), isLoading: false, error: null } as ReturnType<typeof useAuditLog>)
  })

  it('typing in action filter re-queries with action param', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.type(screen.getByPlaceholderText('Filter by action…'), 'user_delete')

    expect(vi.mocked(useAuditLog)).toHaveBeenCalledWith(
      expect.objectContaining({ action: 'user_delete', page: 1 })
    )
  })
})
