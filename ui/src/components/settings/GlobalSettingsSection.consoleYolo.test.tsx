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
    api_native_tools_enabled: false,
    api_via_cli_enabled: false,
    claude_system_prompt_override_enabled: false,
    low_consumption_mode: false,
    simplified_agents_graph: false,
    experimental: false,
    capture_thinking_enabled: false,
    console_yolo: true,
    stall_start_timeout_sec: null,
    stall_running_timeout_sec: null,
    ...overrides,
  }
}

// Toggle DOM order (see GlobalSettingsSection.toggles.test.tsx): [10]=console_yolo (last, appended
// after dynamic_workflow_auto_enabled).
describe('GlobalSettingsSection console_yolo toggle', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders Console yolo mode toggle reflecting server state (true, the default)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ console_yolo: true }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[10]).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByText('Console yolo mode')).toBeInTheDocument()
  })

  it('renders Console yolo mode toggle reflecting server state (false)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ console_yolo: false }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[10]).toHaveAttribute('aria-checked', 'false')
  })

  it('clicking Console yolo mode toggle (false→true) calls updateGlobalSettings({ console_yolo: true })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ console_yolo: false }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[10])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ console_yolo: true })
    })
  })

  it('clicking Console yolo mode toggle (true→false) calls updateGlobalSettings({ console_yolo: false })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ console_yolo: true }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[10])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ console_yolo: false })
    })
  })
})
