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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false })
    renderWithQuery(<GlobalSettingsSection />)
    const toggle = await screen.findByRole('switch')
    expect(toggle).toHaveAttribute('aria-checked', 'false')
  })

  it('renders toggle reflecting server state (true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: true })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false })
    renderWithQuery(<GlobalSettingsSection />)
    await screen.findByRole('switch')
    expect(screen.getByText('Global Settings')).toBeInTheDocument()
    expect(screen.getByText('Low consumption mode')).toBeInTheDocument()
    expect(screen.getByText(/when enabled, agents with a configured alternative/i)).toBeInTheDocument()
  })

  it('clicking toggle calls updateGlobalSettings with toggled value (false → true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: false })
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
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue({ low_consumption_mode: true })
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggle = await screen.findByRole('switch')
    await user.click(toggle)

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ low_consumption_mode: false })
    })
  })
})
