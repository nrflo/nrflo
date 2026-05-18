import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { GlobalSettingsSection } from './GlobalSettingsSection'
import * as settingsApi from '@/api/settings'
import type { GlobalSettings } from '@/api/settings'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/api/settings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/settings')>()
  return { ...actual, getGlobalSettings: vi.fn(), updateGlobalSettings: vi.fn() }
})

function makeSettings(overrides: Partial<GlobalSettings> = {}): GlobalSettings {
  return {
    api_mode_enabled: false,
    low_consumption_mode: false,
    context_save_via_agent: false,
    simplified_agents_graph: false,
    experimental: false,
    stall_start_timeout_sec: null,
    stall_running_timeout_sec: null,
    ...overrides,
  }
}
describe('GlobalSettingsSection', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders toggle reflecting server state (false)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ low_consumption_mode: false }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    // toggles[0]=api_mode, toggles[1]=low_consumption
    expect(toggles[1]).toHaveAttribute('aria-checked', 'false')
  })

  it('renders toggle reflecting server state (true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ low_consumption_mode: true }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[1]).toHaveAttribute('aria-checked', 'true')
  })

  it('shows loading state while fetching', () => {
    vi.mocked(settingsApi.getGlobalSettings).mockReturnValue(new Promise(() => {}))
    renderWithQuery(<GlobalSettingsSection />)
    expect(screen.getByText('Loading settings...')).toBeInTheDocument()
  })

  it('shows error when fetch fails', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockRejectedValue(new Error('Network error'))
    renderWithQuery(<GlobalSettingsSection />)
    expect(await screen.findByText(/Error: Network error/)).toBeInTheDocument()
  })

  it('renders section title and field label', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    renderWithQuery(<GlobalSettingsSection />)
    await screen.findAllByRole('switch')
    expect(screen.getByText('Global Settings')).toBeInTheDocument()
    expect(screen.getByText('Low consumption mode')).toBeInTheDocument()
    expect(screen.getByText(/when enabled, agents with a configured alternative/i)).toBeInTheDocument()
    expect(screen.queryByText('Workflow session retention limit')).not.toBeInTheDocument()
  })

  it('clicking toggle calls updateGlobalSettings with toggled value (false → true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ low_consumption_mode: false }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    // toggles[1] = low_consumption (toggles[0] = api_mode)
    await user.click(toggles[1])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ low_consumption_mode: true })
    })
  })

  it('clicking toggle when true sends false', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ low_consumption_mode: true }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[1])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ low_consumption_mode: false })
    })
  })

  it('renders stall start input empty when server returns null', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    renderWithQuery(<GlobalSettingsSection />)
    const input = await screen.findByPlaceholderText('120')
    expect(input).toHaveValue('')
    expect(screen.getByText('Stall start timeout (sec)')).toBeInTheDocument()
  })

  it('renders stall start input with numeric server value', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ stall_start_timeout_sec: 60 }))
    renderWithQuery(<GlobalSettingsSection />)
    const input = await screen.findByPlaceholderText('120')
    expect(input).toHaveValue('60')
  })

  it('blur with positive value calls updateGlobalSettings with stall_start_timeout_sec', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const input = await screen.findByPlaceholderText('120')
    await user.type(input, '60')
    await user.tab()

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ stall_start_timeout_sec: 60 })
    })
  })

  it('blur with "0" calls updateGlobalSettings with stall_start_timeout_sec: 0 (disabled)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const input = await screen.findByPlaceholderText('120')
    await user.type(input, '0')
    await user.tab()

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ stall_start_timeout_sec: 0 })
    })
  })

  it('blur with empty string calls updateGlobalSettings with stall_start_timeout_sec: null', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ stall_start_timeout_sec: 60 }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const input = await screen.findByPlaceholderText('120')
    await user.clear(input)
    await user.tab()

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ stall_start_timeout_sec: null })
    })
  })

  it('negative stall start value resets to server value without calling API', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ stall_start_timeout_sec: 60 }))
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const input = await screen.findByPlaceholderText('120')
    await user.clear(input)
    await user.type(input, '-5')
    await user.tab()

    await waitFor(() => { expect(input).toHaveValue('60') })
    expect(settingsApi.updateGlobalSettings).not.toHaveBeenCalled()
  })

  it('Enter key submits stall start value', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const input = await screen.findByPlaceholderText('120')
    await user.type(input, '90')
    await user.keyboard('{Enter}')

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ stall_start_timeout_sec: 90 })
    })
  })

  it('shows stall start tooltip with two paragraphs including Claude CLI bug note', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    renderWithQuery(<GlobalSettingsSection />)

    await screen.findByPlaceholderText('120')
    const stallStartLabel = screen.getByText('Stall start timeout (sec)')
    const tooltipTrigger = stallStartLabel.closest('.flex')?.querySelector('[data-state]') as HTMLElement

    const user = userEvent.setup()
    await user.hover(tooltipTrigger)

    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent(/Time before first agent message/)
    expect(tooltip).toHaveTextContent(/drops tool_use blocks from streaming API responses/)
    expect(tooltip).toHaveTextContent(/anthropics\/claude-code#25979/)
  })

  it('renders stall running input with server value and submits on blur', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ stall_running_timeout_sec: 300 }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const input = await screen.findByPlaceholderText('480')
    expect(input).toHaveValue('300')
    expect(screen.getByText('Stall running timeout (sec)')).toBeInTheDocument()

    await user.clear(input)
    await user.type(input, '600')
    await user.tab()

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ stall_running_timeout_sec: 600 })
    })
  })

  it('renders API mode toggle reflecting server state (false)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ api_mode_enabled: false }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[0]).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByText('Enable API mode')).toBeInTheDocument()
  })

  it('renders API mode toggle reflecting server state (true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ api_mode_enabled: true }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[0]).toHaveAttribute('aria-checked', 'true')
  })

  it('clicking API mode toggle (false→true) calls updateGlobalSettings({ api_mode_enabled: true })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ api_mode_enabled: false }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[0])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ api_mode_enabled: true })
    })
  })

  it('clicking API mode toggle (true→false) calls updateGlobalSettings({ api_mode_enabled: false })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ api_mode_enabled: true }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[0])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ api_mode_enabled: false })
    })
  })

  it('renders Experimental features toggle reflecting server state (false)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ experimental: false }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    // toggles[0]=api_mode, [1]=low_consumption, [2]=context_save, [3]=simplified_graph, [4]=experimental
    expect(toggles[4]).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByText('Experimental features')).toBeInTheDocument()
  })

  it('renders Experimental features toggle reflecting server state (true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ experimental: true }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[4]).toHaveAttribute('aria-checked', 'true')
  })

  it('clicking Experimental toggle calls updateGlobalSettings({ experimental: true })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ experimental: false }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[4])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ experimental: true })
    })
  })

  it('clicking Experimental toggle when true sends false', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ experimental: true }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[4])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ experimental: false })
    })
  })

})
