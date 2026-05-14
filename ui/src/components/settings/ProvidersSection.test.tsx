import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProvidersSection } from './ProvidersSection'
import * as settingsApi from '@/api/settings'
import { renderWithQuery } from '@/test/utils'

vi.mock('./ProviderModelsList', () => ({
  ProviderModelsList: ({ provider }: { provider: string }) => (
    <div data-testid="provider-models-list" data-provider={provider} />
  ),
}))
vi.mock('@/api/settings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/settings')>()
  return { ...actual, getGlobalSettings: vi.fn(), updateGlobalSettings: vi.fn() }
})

describe('Sync Claude limits', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({
      api_mode_enabled: false,
      low_consumption_mode: false,
      context_save_via_agent: false,
      simplified_agents_graph: false,
      experimental: false,
      sync_claude_limits: false,
      session_retention_limit: 1000,
      stall_start_timeout_sec: null,
      stall_running_timeout_sec: null,
    })
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
  })

  it('renders toggle with aria-checked=false when activeProvider=claude and sync_claude_limits=false', async () => {
    renderWithQuery(<ProvidersSection activeProvider="claude" />)
    const label = await screen.findByText('Sync Claude limits')
    const row = label.closest('.flex')!
    expect(within(row).getByRole('switch')).toHaveAttribute('aria-checked', 'false')
  })

  it('clicking toggle calls updateGlobalSettings({sync_claude_limits:true}) when current is false', async () => {
    renderWithQuery(<ProvidersSection activeProvider="claude" />)
    const label = await screen.findByText('Sync Claude limits')
    const toggle = within(label.closest('.flex')!).getByRole('switch')
    await userEvent.setup().click(toggle)
    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ sync_claude_limits: true })
    })
  })

  it('clicking toggle calls updateGlobalSettings({sync_claude_limits:false}) when current is true', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({
      api_mode_enabled: false,
      low_consumption_mode: false,
      context_save_via_agent: false,
      simplified_agents_graph: false,
      experimental: false,
      sync_claude_limits: true,
      session_retention_limit: 1000,
      stall_start_timeout_sec: null,
      stall_running_timeout_sec: null,
    })
    renderWithQuery(<ProvidersSection activeProvider="claude" />)
    const label = await screen.findByText('Sync Claude limits')
    const toggle = within(label.closest('.flex')!).getByRole('switch')
    await waitFor(() => expect(toggle).toHaveAttribute('aria-checked', 'true'))
    await userEvent.setup().click(toggle)
    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ sync_claude_limits: false })
    })
  })

  it('Claude limits card not in DOM when activeProvider=opencode', () => {
    renderWithQuery(<ProvidersSection activeProvider="opencode" />)
    expect(screen.queryByText('Claude limits sync')).not.toBeInTheDocument()
  })

  it('Claude limits card not in DOM when activeProvider=codex', () => {
    renderWithQuery(<ProvidersSection activeProvider="codex" />)
    expect(screen.queryByText('Claude limits sync')).not.toBeInTheDocument()
  })
})

describe('ProvidersSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({
      api_mode_enabled: false,
      low_consumption_mode: false,
      context_save_via_agent: false,
      simplified_agents_graph: false,
      experimental: false,
      sync_claude_limits: false,
      session_retention_limit: 1000,
      stall_start_timeout_sec: null,
      stall_running_timeout_sec: null,
    })
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
  })

  it('renders ProviderModelsList with active provider prop', () => {
    renderWithQuery(<ProvidersSection activeProvider="claude" />)
    expect(screen.getByTestId('provider-models-list')).toHaveAttribute('data-provider', 'claude')
  })

  it('passes opencode provider prop to ProviderModelsList', () => {
    renderWithQuery(<ProvidersSection activeProvider="opencode" />)
    expect(screen.getByTestId('provider-models-list')).toHaveAttribute('data-provider', 'opencode')
  })

  it('Modes card is absent for all providers', () => {
    const { unmount } = renderWithQuery(<ProvidersSection activeProvider="claude" />)
    expect(screen.queryByText(/\bModes\b/)).not.toBeInTheDocument()
    unmount()

    renderWithQuery(<ProvidersSection activeProvider="opencode" />)
    expect(screen.queryByText(/\bModes\b/)).not.toBeInTheDocument()
  })
})
