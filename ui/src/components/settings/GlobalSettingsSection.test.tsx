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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, session_retention_limit: 100 })
    renderWithQuery(<GlobalSettingsSection />)
    const toggle = await screen.findByRole('switch')
    expect(toggle).toHaveAttribute('aria-checked', 'false')
  })

  it('renders toggle reflecting server state (true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: true, session_retention_limit: 100 })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, session_retention_limit: 100 })
    renderWithQuery(<GlobalSettingsSection />)
    await screen.findByRole('switch')
    expect(screen.getByText('Global Settings')).toBeInTheDocument()
    expect(screen.getByText('Low consumption mode')).toBeInTheDocument()
    expect(screen.getByText(/when enabled, agents with a configured alternative/i)).toBeInTheDocument()
  })

  it('clicking toggle calls updateGlobalSettings with toggled value (false → true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, session_retention_limit: 100 })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: true, session_retention_limit: 100 })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, session_retention_limit: 100 })
    renderWithQuery(<GlobalSettingsSection />)
    const input = await screen.findByRole('spinbutton')
    expect(input).toHaveValue(100)
    expect(screen.getByText('Session retention limit')).toBeInTheDocument()
  })

  it('blur with valid new value calls updateGlobalSettings with session_retention_limit', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, session_retention_limit: 100 })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, session_retention_limit: 100 })
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const input = await screen.findByRole('spinbutton')
    await user.clear(input)
    await user.type(input, '5')
    await user.tab()

    await waitFor(() => {
      expect(input).toHaveValue(100)
    })
    expect(settingsApi.updateGlobalSettings).not.toHaveBeenCalled()
  })

  it('Enter key submits valid retention value', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, session_retention_limit: 100 })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false, session_retention_limit: 100 })
    renderWithQuery(<GlobalSettingsSection />)

    await screen.findByRole('spinbutton')
    const retentionLabel = screen.getByText('Session retention limit')
    const tooltipTrigger = retentionLabel.parentElement?.lastElementChild as HTMLElement

    const user = userEvent.setup()
    await user.hover(tooltipTrigger)

    expect(screen.getByText(/Maximum number of completed agent sessions/)).toBeInTheDocument()
    expect(screen.getByText(/Associated agent messages are automatically removed/)).toBeInTheDocument()
  })
})
