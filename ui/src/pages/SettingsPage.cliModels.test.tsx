import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/utils'
import { SettingsPage } from './SettingsPage'

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

vi.mock('@/api/settings', () => ({
  settingsKeys: { all: ['settings'], global: () => ['settings', 'global'] },
  getGlobalSettings: vi.fn().mockResolvedValue({
    low_consumption_mode: false,
    api_mode_enabled: false,
    sync_claude_limits: false,
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

vi.mock('@/components/settings/ProvidersSection', () => ({
  ProvidersSection: ({ activeProvider }: { activeProvider: string }) => (
    <div data-testid="providers-section" data-provider={activeProvider} />
  ),
}))

vi.mock('@/components/settings/UsersSection', () => ({
  UsersSection: () => <h2>Users</h2>,
}))

vi.mock('@/components/settings/AuditLogSection', () => ({
  AuditLogSection: () => <h2>Audit Log</h2>,
}))

function renderPage(search = '') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={[`/settings${search}`]}>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SettingsPage — CLI Models tab', () => {
  beforeEach(() => vi.clearAllMocks())

  it('CLI Models tab appears between Default Templates and Logs', () => {
    renderPage()
    const tabLabels = ['General', 'Projects', 'System Agents', 'Default Templates', 'CLI Models', 'Logs', 'Administration']
    const tabs = screen
      .getAllByRole('button')
      .filter((b) => tabLabels.includes(b.textContent ?? ''))
    const labels = tabs.map((t) => t.textContent)
    const dtIdx = labels.indexOf('Default Templates')
    const cliIdx = labels.indexOf('CLI Models')
    const logsIdx = labels.indexOf('Logs')
    expect(cliIdx).toBeGreaterThan(dtIdx)
    expect(cliIdx).toBeLessThan(logsIdx)
  })

  it('navigating to cli-models tab renders ProvidersSection', async () => {
    renderPage('?tab=logs')
    await userEvent.click(screen.getByRole('button', { name: 'CLI Models' }))
    expect(screen.getByTestId('providers-section')).toBeInTheDocument()
  })

  it('deep-link ?tab=cli-models renders ProvidersSection with claude default', () => {
    renderPage('?tab=cli-models')
    expect(screen.getByTestId('providers-section')).toHaveAttribute('data-provider', 'claude')
  })

  it('deep-link ?tab=cli-models&sub=gemini passes gemini to ProvidersSection', () => {
    renderPage('?tab=cli-models&sub=gemini')
    expect(screen.getByTestId('providers-section')).toHaveAttribute('data-provider', 'gemini')
  })

  it('provider sub-tab strip renders Claude/OpenCode/Codex/Gemini in order', () => {
    renderPage('?tab=cli-models')
    const providerBtns = ['Claude', 'OpenCode', 'Codex', 'Gemini']
    const buttons = screen
      .getAllByRole('button')
      .filter((b) => providerBtns.includes(b.textContent ?? ''))
    expect(buttons.map((b) => b.textContent)).toEqual(['Claude', 'OpenCode', 'Codex', 'Gemini'])
  })

  it('clicking a provider sub-tab updates the active provider', async () => {
    renderPage('?tab=cli-models')
    await userEvent.click(screen.getByRole('button', { name: 'Gemini' }))
    expect(screen.getByTestId('providers-section')).toHaveAttribute('data-provider', 'gemini')
  })

  it('provider sub-tab strip only appears on cli-models tab', () => {
    renderPage('?tab=logs')
    expect(screen.queryByRole('button', { name: 'OpenCode' })).not.toBeInTheDocument()
  })

  it('administration sub=audit → cli-models → administration falls back to users', async () => {
    renderPage('?tab=administration&sub=audit')
    expect(screen.getByRole('heading', { name: 'Audit Log' })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'CLI Models' }))
    expect(screen.getByTestId('providers-section')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Administration' }))
    expect(screen.getByRole('heading', { name: 'Users' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Audit Log' })).not.toBeInTheDocument()
  })
})
