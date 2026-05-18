import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/utils'
import { SettingsPage } from './SettingsPage'

vi.mock('@/hooks/useUsers')
vi.mock('@/hooks/useAuditLog')
vi.mock('@/api/settings', () => ({
  settingsKeys: { all: ['settings'], global: () => ['settings', 'global'] },
  getGlobalSettings: vi.fn().mockResolvedValue({
    low_consumption_mode: false,
    api_mode_enabled: false,
    simplified_agents_graph: false,
  }),
  updateGlobalSettings: vi.fn(),
}))
vi.mock('@/api/systemAgentDefs', () => ({
  listSystemAgentDefs: vi.fn().mockResolvedValue([]),
  createSystemAgentDef: vi.fn(),
  updateSystemAgentDef: vi.fn(),
  deleteSystemAgentDef: vi.fn(),
}))
vi.mock('@/hooks/useLogs', () => ({
  useLogs: vi.fn().mockReturnValue({ data: undefined, isLoading: true, error: undefined, refetch: vi.fn() }),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector?: (s: object) => unknown) => {
    const store = {
      currentProject: 'p1',
      setCurrentProject: vi.fn(),
      loadProjects: vi.fn(),
      projects: [{ id: 'p1' }],
      projectsLoaded: true,
    }
    return selector ? selector(store) : store
  },
}))

import { useUsers, useDeleteUser, useCreateUser, useUpdateUser, useResetUserPassword } from '@/hooks/useUsers'
import { useAuditLog } from '@/hooks/useAuditLog'

function setupMocks() {
  vi.mocked(useUsers).mockReturnValue({ data: { users: [] }, isLoading: false, error: undefined } as any)
  vi.mocked(useDeleteUser).mockReturnValue({ mutateAsync: vi.fn(), isPending: false } as any)
  vi.mocked(useCreateUser).mockReturnValue({ mutateAsync: vi.fn(), isPending: false } as any)
  vi.mocked(useUpdateUser).mockReturnValue({ mutateAsync: vi.fn(), isPending: false } as any)
  vi.mocked(useResetUserPassword).mockReturnValue({ mutateAsync: vi.fn(), isPending: false } as any)
  vi.mocked(useAuditLog).mockReturnValue({
    isLoading: false,
    error: undefined,
    data: { items: [], total: 0, page: 1, per_page: 50 },
  } as any)
}

function renderPage(search = '') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={[`/settings${search}`]}>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SettingsPage - Administration tab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setupMocks()
  })

  it('defaults to Users sub-tab when navigating to administration tab', () => {
    renderPage('?tab=administration')
    expect(screen.getByRole('heading', { name: 'Users' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Audit Log' })).not.toBeInTheDocument()
  })

  it('clicking Audit Log sub-tab renders AuditLogSection', async () => {
    renderPage('?tab=administration&sub=users')
    expect(screen.getByRole('heading', { name: 'Users' })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Audit Log' }))
    expect(screen.getByRole('heading', { name: 'Audit Log' })).toBeInTheDocument()
    expect(screen.getByText('No audit entries found.')).toBeInTheDocument()
  })

  it('initial route with sub=audit renders AuditLogSection', () => {
    renderPage('?tab=administration&sub=audit')
    expect(screen.getByRole('heading', { name: 'Audit Log' })).toBeInTheDocument()
    expect(screen.getByText('No audit entries found.')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Users' })).not.toBeInTheDocument()
  })

  it('clicking Administration main tab from another tab shows Users sub-tab', async () => {
    // Start on the logs tab to avoid needing GlobalSettingsSection mocks
    renderPage('?tab=logs')
    await userEvent.click(screen.getByRole('button', { name: 'Administration' }))
    expect(screen.getByRole('heading', { name: 'Users' })).toBeInTheDocument()
  })

  it('sub-tab strip only appears when on administration tab', () => {
    renderPage('?tab=administration&sub=users')
    expect(screen.getByRole('button', { name: 'Users' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Audit Log' })).toBeInTheDocument()
  })
})
