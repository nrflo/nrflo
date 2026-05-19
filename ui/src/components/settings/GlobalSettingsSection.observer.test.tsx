import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { GlobalSettingsSection } from './GlobalSettingsSection'
import * as settingsApi from '@/api/settings'
import type { GlobalSettings } from '@/api/settings'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/api/settings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/settings')>()
  return { ...actual, getGlobalSettings: vi.fn(), updateGlobalSettings: vi.fn() }
})

vi.mock('@/hooks/useCLIModels', () => ({
  useCLIModels: () => ({
    data: [
      { id: 'claude-sonnet', cli_type: 'claude', display_name: 'Sonnet', enabled: true },
    ],
  }),
}))

function makeSettings(overrides: Partial<GlobalSettings> = {}): GlobalSettings {
  return {
    api_mode_enabled: false,
    low_consumption_mode: false,
    context_save_via_agent: false,
    simplified_agents_graph: false,
    experimental: false,
    experimental_observer_enabled: false,
    observer_system_context: '',
    observer_provider: '',
    observer_model: '',
    stall_start_timeout_sec: null,
    stall_running_timeout_sec: null,
    ...overrides,
  }
}

describe('GlobalSettingsSection — observer settings', () => {
  beforeEach(() => vi.clearAllMocks())

  it('observer_system_context textarea is absent when experimental_observer_enabled is false', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ experimental_observer_enabled: false }))
    renderWithQuery(<GlobalSettingsSection />)
    await screen.findAllByRole('switch')
    expect(screen.queryByPlaceholderText(/system context for observer agents/i)).not.toBeInTheDocument()
  })

  it('observer toggle is present regardless of flag value', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ experimental_observer_enabled: false }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    // api_mode[0], low_consumption[1], context_save[2], simplified_graph[3], experimental[4], observer_mode[5]
    expect(toggles[5]).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByText('Observer mode')).toBeInTheDocument()
  })

  it('observer_system_context textarea appears when experimental_observer_enabled is true', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ experimental_observer_enabled: true }))
    renderWithQuery(<GlobalSettingsSection />)
    await screen.findAllByRole('switch')
    expect(screen.getByPlaceholderText(/system context for observer agents/i)).toBeInTheDocument()
  })

  it('toggling observer mode on calls updateGlobalSettings({ experimental_observer_enabled: true })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ experimental_observer_enabled: false }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[5])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ experimental_observer_enabled: true })
    })
  })

  it('toggling observer mode off calls updateGlobalSettings({ experimental_observer_enabled: false })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ experimental_observer_enabled: true }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[5])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ experimental_observer_enabled: false })
    })
  })

  it('changing observer_system_context textarea calls updateGlobalSettings with observer_system_context', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(
      makeSettings({ experimental_observer_enabled: true, observer_system_context: '' })
    )
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const textarea = await screen.findByPlaceholderText(/system context for observer agents/i)
    fireEvent.change(textarea, { target: { value: 'Watch for errors' } })

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith(
        expect.objectContaining({ observer_system_context: 'Watch for errors' })
      )
    })
  })
})
