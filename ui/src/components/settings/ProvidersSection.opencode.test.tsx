import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within } from '@testing-library/react'
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

function setup(activeProvider: string, modes: string[]) {
  vi.mocked(useProviders).mockReturnValue({
    data: { [activeProvider]: { modes } },
    isLoading: false,
    error: null,
  } as never)
  vi.mocked(useUpdateProvider).mockReturnValue({ mutate: mockMutate, isPending: false } as never)
}

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

describe('ProvidersSection — opencode tab', () => {
  it('renders static note instead of Toggle rows', () => {
    setup('opencode', ['cli'])
    renderWithQuery(<ProvidersSection activeProvider="opencode" />)
    expect(screen.getByText('opencode runs cli mode only by design.')).toBeInTheDocument()
  })

  it('Modes card contains no Toggle (switch) elements', () => {
    setup('opencode', ['cli'])
    renderWithQuery(<ProvidersSection activeProvider="opencode" />)
    const modesHeading = screen.getByText('Modes')
    const modesCard = modesHeading.closest('.rounded-lg, [class*="card"], div')!
    // Walk up to the card container that wraps both header and content
    const cardRoot = modesHeading.closest('div[class]')!.parentElement!.parentElement!
    expect(within(cardRoot).queryAllByRole('switch')).toHaveLength(0)
  })

  it('Modes card title is still present', () => {
    setup('opencode', ['cli'])
    renderWithQuery(<ProvidersSection activeProvider="opencode" />)
    expect(screen.getByText('Modes')).toBeInTheDocument()
  })

  it('queryAllByRole switch returns empty across entire document', () => {
    setup('opencode', ['cli'])
    renderWithQuery(<ProvidersSection activeProvider="opencode" />)
    // opencode has no Sync Claude limits card either, so no switches anywhere
    expect(screen.queryAllByRole('switch')).toHaveLength(0)
  })

  it('ProviderModelsList still receives opencode provider', () => {
    setup('opencode', ['cli'])
    renderWithQuery(<ProvidersSection activeProvider="opencode" />)
    expect(screen.getByTestId('provider-models-list')).toHaveAttribute('data-provider', 'opencode')
  })
})

describe('ProvidersSection — codex tab', () => {
  it('renders both cli and cli_interactive Toggle rows', () => {
    setup('codex', ['cli', 'cli_interactive'])
    renderWithQuery(<ProvidersSection activeProvider="codex" />)
    const switches = screen.getAllByRole('switch')
    expect(switches).toHaveLength(2)
  })

  it('cli Toggle is checked when cli is in modes', () => {
    setup('codex', ['cli'])
    renderWithQuery(<ProvidersSection activeProvider="codex" />)
    const [cliSwitch] = screen.getAllByRole('switch')
    expect(cliSwitch).toHaveAttribute('aria-checked', 'true')
  })

  it('cli_interactive Toggle is unchecked when not in modes', () => {
    setup('codex', ['cli'])
    renderWithQuery(<ProvidersSection activeProvider="codex" />)
    const [, cliInteractiveSwitch] = screen.getAllByRole('switch')
    expect(cliInteractiveSwitch).toHaveAttribute('aria-checked', 'false')
  })
})
