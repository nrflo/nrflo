import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { GlobalSettingsSection } from './GlobalSettingsSection'
import * as settingsApi from '@/api/settings'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/api/settings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/settings')>()
  return { ...actual, getGlobalSettings: vi.fn(), updateGlobalSettings: vi.fn() }
})

describe('GlobalSettingsSection', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders toggle reflecting server state (false)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
    renderWithQuery(<GlobalSettingsSection />)
    const toggle = await screen.findByRole('switch')
    expect(toggle).toHaveAttribute('aria-checked', 'false')
  })

  it('renders toggle reflecting server state (true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: true, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
    renderWithQuery(<GlobalSettingsSection />)
    const toggle = await screen.findByRole('switch')
    expect(toggle).toHaveAttribute('aria-checked', 'true')
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
    renderWithQuery(<GlobalSettingsSection />)
    await screen.findByRole('switch')
    expect(screen.getByText('Global Settings')).toBeInTheDocument()
    expect(screen.getByText('Low consumption mode')).toBeInTheDocument()
    expect(screen.getByText(/when enabled, agents with a configured alternative/i)).toBeInTheDocument()
  })

  it('clicking toggle calls updateGlobalSettings with toggled value (false → true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggle = await screen.findByRole('switch')
    await user.click(toggle)

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ low_consumption_mode: true })
    })
  })

  it('clicking toggle when true sends false', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: true, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggle = await screen.findByRole('switch')
    await user.click(toggle)

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ low_consumption_mode: false })
    })
  })

  it('renders number input with server value and label', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
    renderWithQuery(<GlobalSettingsSection />)
    const input = await screen.findByRole('spinbutton')
    expect(input).toHaveValue(1000)
    expect(screen.getByText('Session retention limit')).toBeInTheDocument()
  })

  it('blur with valid new value calls updateGlobalSettings with session_retention_limit', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const input = await screen.findByRole('spinbutton')
    await user.clear(input)
    await user.type(input, '50')
    await user.tab()

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ session_retention_limit: 50 })
    })
  })

  it('blur with value below minimum resets to server value without calling API', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const input = await screen.findByRole('spinbutton')
    await user.clear(input)
    await user.type(input, '5')
    await user.tab()

    await waitFor(() => {
      expect(input).toHaveValue(1000)
    })
    expect(settingsApi.updateGlobalSettings).not.toHaveBeenCalled()
  })

  it('Enter key submits valid retention value', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const input = await screen.findByRole('spinbutton')
    await user.clear(input)
    await user.type(input, '75')
    await user.keyboard('{Enter}')

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ session_retention_limit: 75 })
    })
  })

  it('shows tooltip text on hover over info icon', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
    renderWithQuery(<GlobalSettingsSection />)

    await screen.findByRole('spinbutton')
    const retentionLabel = screen.getByText('Session retention limit')
    // Radix Tooltip.Trigger renders with data-state; find the trigger span near the label
    const tooltipTrigger = retentionLabel.closest('.flex')?.querySelector('[data-state]') as HTMLElement

    const user = userEvent.setup()
    await user.hover(tooltipTrigger)

    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent(/Maximum number of completed agent sessions/)
    expect(tooltip).toHaveTextContent(/Associated agent messages are automatically removed/)
  })

  it('renders stall start input empty when server returns null', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
    renderWithQuery(<GlobalSettingsSection />)
    const input = await screen.findByPlaceholderText('120')
    expect(input).toHaveValue('')
    expect(screen.getByText('Stall start timeout (sec)')).toBeInTheDocument()
  })

  it('renders stall start input with numeric server value', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: 60, stall_running_timeout_sec: null })
    renderWithQuery(<GlobalSettingsSection />)
    const input = await screen.findByPlaceholderText('120')
    expect(input).toHaveValue('60')
  })

  it('blur with positive value calls updateGlobalSettings with stall_start_timeout_sec', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: 60, stall_running_timeout_sec: null })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: 60, stall_running_timeout_sec: null })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: null })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, context_save_via_agent: false, session_retention_limit: 1000, stall_start_timeout_sec: null, stall_running_timeout_sec: 300 })
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
})
