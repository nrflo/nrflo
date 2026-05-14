import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProvidersSection } from './ProvidersSection'
import { useProviders, useUpdateProvider } from '@/hooks/useProviders'
import * as settingsApi from '@/api/settings'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/hooks/useProviders')
vi.mock('./ProviderModelsList', () => ({
  ProviderModelsList: ({ provider }: { provider: string }) => (
    <div data-testid="provider-models-list" data-provider={provider} />
  ),
}))
vi.mock('@/api/settings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/settings')>()
  return { ...actual, getGlobalSettings: vi.fn(), updateGlobalSettings: vi.fn() }
})

const mockMutate = vi.fn()

function setup(activeProvider: string, modes: string[], { isPending = false } = {}) {
  vi.mocked(useProviders).mockReturnValue({
    data: { [activeProvider]: { modes } },
    isLoading: false,
    error: null,
  } as never)
  vi.mocked(useUpdateProvider).mockReturnValue({ mutate: mockMutate, isPending } as never)
}

describe('Sync Claude limits', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockMutate.mockReset()
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
    setup('claude', ['cli_interactive'])
    renderWithQuery(<ProvidersSection activeProvider="claude" />)
    const label = await screen.findByText('Sync Claude limits')
    const row = label.closest('.flex')!
    expect(within(row).getByRole('switch')).toHaveAttribute('aria-checked', 'false')
  })

  it('clicking toggle calls updateGlobalSettings({sync_claude_limits:true}) when current is false', async () => {
    setup('claude', ['cli_interactive'])
    renderWithQuery(<ProvidersSection activeProvider="claude" />)
    const label = await screen.findByText('Sync Claude limits')
    const toggle = within(label.closest('.flex')!).getByRole('switch')
    await userEvent.setup().click(toggle)
    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ sync_claude_limits: true })
    })
  })

  it('clicking toggle calls updateGlobalSettings({sync_claude_limits:false}) when current is true', async () => {
    setup('claude', ['cli_interactive'])
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
    setup('opencode', ['cli'])
    renderWithQuery(<ProvidersSection activeProvider="opencode" />)
    expect(screen.queryByText('Claude limits sync')).not.toBeInTheDocument()
  })

  it('Claude limits card not in DOM when activeProvider=codex', () => {
    setup('codex', ['cli'])
    renderWithQuery(<ProvidersSection activeProvider="codex" />)
    expect(screen.queryByText('Claude limits sync')).not.toBeInTheDocument()
  })
})

describe('ProvidersSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockMutate.mockReset()
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

  it('shows loading state while fetching', () => {
    vi.mocked(useProviders).mockReturnValue({ isLoading: true, data: undefined, error: null } as never)
    vi.mocked(useUpdateProvider).mockReturnValue({ mutate: mockMutate, isPending: false } as never)
    renderWithQuery(<ProvidersSection activeProvider="claude" />)
    expect(screen.getByText(/loading providers/i)).toBeInTheDocument()
  })

  it('shows error state', () => {
    vi.mocked(useProviders).mockReturnValue({ isLoading: false, data: undefined, error: new Error('fetch failed') } as never)
    vi.mocked(useUpdateProvider).mockReturnValue({ mutate: mockMutate, isPending: false } as never)
    renderWithQuery(<ProvidersSection activeProvider="claude" />)
    expect(screen.getByText(/Error: fetch failed/)).toBeInTheDocument()
  })

  describe('Modes toggles', () => {
    it('both toggles checked when modes includes cli and cli_interactive', () => {
      setup('claude', ['cli', 'cli_interactive'])
      renderWithQuery(<ProvidersSection activeProvider="claude" />)
      const [cliSwitch, cliInteractiveSwitch] = screen.getAllByRole('switch')
      expect(cliSwitch).toHaveAttribute('aria-checked', 'true')
      expect(cliInteractiveSwitch).toHaveAttribute('aria-checked', 'true')
    })

    it('toggles reflect individual enabled states', () => {
      setup('claude', ['cli_interactive'])
      renderWithQuery(<ProvidersSection activeProvider="claude" />)
      const [cliSwitch, cliInteractiveSwitch] = screen.getAllByRole('switch')
      expect(cliSwitch).toHaveAttribute('aria-checked', 'false')
      expect(cliInteractiveSwitch).toHaveAttribute('aria-checked', 'true')
    })

    it('toggling cli off when both enabled calls mutate with remaining mode', async () => {
      setup('claude', ['cli', 'cli_interactive'])
      renderWithQuery(<ProvidersSection activeProvider="claude" />)
      const [cliSwitch] = screen.getAllByRole('switch')
      await userEvent.setup().click(cliSwitch)
      expect(mockMutate).toHaveBeenCalledWith({ name: 'claude', modes: ['cli_interactive'] })
    })

    it('toggling cli_interactive off when both enabled calls mutate with [cli]', async () => {
      setup('claude', ['cli', 'cli_interactive'])
      renderWithQuery(<ProvidersSection activeProvider="claude" />)
      const [, cliInteractiveSwitch] = screen.getAllByRole('switch')
      await userEvent.setup().click(cliInteractiveSwitch)
      expect(mockMutate).toHaveBeenCalledWith({ name: 'claude', modes: ['cli'] })
    })

    it('toggling the only enabled mode off shows error and skips mutate', async () => {
      setup('claude', ['cli'])
      renderWithQuery(<ProvidersSection activeProvider="claude" />)
      const [cliSwitch] = screen.getAllByRole('switch')
      await userEvent.setup().click(cliSwitch)
      expect(mockMutate).not.toHaveBeenCalled()
      expect(screen.getByText('At least one mode must be enabled')).toBeInTheDocument()
    })

    it('toggling when provider has no saved modes still validates', async () => {
      setup('opencode', [])
      renderWithQuery(<ProvidersSection activeProvider="opencode" />)
      const [cliSwitch] = screen.getAllByRole('switch')
      // enabling a mode when none enabled calls mutate (nextModes = [cli])
      await userEvent.setup().click(cliSwitch)
      expect(mockMutate).toHaveBeenCalledWith({ name: 'opencode', modes: ['cli'] })
    })
  })

  describe('Billing banner', () => {
    it('visible when activeProvider=claude and cli is enabled', () => {
      setup('claude', ['cli'])
      renderWithQuery(<ProvidersSection activeProvider="claude" />)
      expect(screen.getByText(/Claude Code CLI.*billed at API rate/)).toBeInTheDocument()
    })

    it('hidden when activeProvider=claude but cli is not enabled', () => {
      setup('claude', ['cli_interactive'])
      renderWithQuery(<ProvidersSection activeProvider="claude" />)
      expect(screen.queryByText(/Claude Code CLI.*billed at API rate/)).not.toBeInTheDocument()
    })

    it('hidden when activeProvider=opencode even with cli enabled', () => {
      setup('opencode', ['cli'])
      renderWithQuery(<ProvidersSection activeProvider="opencode" />)
      expect(screen.queryByText(/Claude Code CLI.*billed at API rate/)).not.toBeInTheDocument()
    })

    it('hidden when activeProvider=codex even with cli enabled', () => {
      setup('codex', ['cli'])
      renderWithQuery(<ProvidersSection activeProvider="codex" />)
      expect(screen.queryByText(/Claude Code CLI.*billed at API rate/)).not.toBeInTheDocument()
    })
  })

  it('renders ProviderModelsList with active provider prop', () => {
    setup('claude', ['cli'])
    renderWithQuery(<ProvidersSection activeProvider="claude" />)
    expect(screen.getByTestId('provider-models-list')).toHaveAttribute('data-provider', 'claude')
  })

  it('passes opencode provider prop to ProviderModelsList', () => {
    setup('opencode', ['cli'])
    renderWithQuery(<ProvidersSection activeProvider="opencode" />)
    expect(screen.getByTestId('provider-models-list')).toHaveAttribute('data-provider', 'opencode')
  })
})
