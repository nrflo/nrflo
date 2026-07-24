import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
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
    api_via_cli_enabled: false,
    claude_system_prompt_override_enabled: false,
    low_consumption_mode: false,
    simplified_agents_graph: false,
    experimental: false,
    capture_thinking_enabled: false,
    stall_start_timeout_sec: null,
    stall_running_timeout_sec: null,
    dynamic_workflow_auto_enabled: false,
    console_yolo: true,
    ...overrides,
  }
}

function dynamicToggleRow() {
  const label = screen.getByText('Allow dynamic_workflow mode=auto')
  return label.closest('div.flex.items-center.justify-between') as HTMLElement
}

describe('GlobalSettingsSection — dynamic workflow auto toggle', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reflects server state (false)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ dynamic_workflow_auto_enabled: false }))
    renderWithQuery(<GlobalSettingsSection />)

    await screen.findByText('Allow dynamic_workflow mode=auto')
    expect(within(dynamicToggleRow()).getByRole('switch')).toHaveAttribute('aria-checked', 'false')
  })

  it('reflects server state (true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ dynamic_workflow_auto_enabled: true }))
    renderWithQuery(<GlobalSettingsSection />)

    await screen.findByText('Allow dynamic_workflow mode=auto')
    expect(within(dynamicToggleRow()).getByRole('switch')).toHaveAttribute('aria-checked', 'true')
  })

  it('clicking the toggle (false→true) PATCHes dynamic_workflow_auto_enabled: true', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ dynamic_workflow_auto_enabled: false }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    await screen.findByText('Allow dynamic_workflow mode=auto')
    await user.click(within(dynamicToggleRow()).getByRole('switch'))

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ dynamic_workflow_auto_enabled: true })
    })
  })

  it('clicking the toggle (true→false) PATCHes dynamic_workflow_auto_enabled: false', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ dynamic_workflow_auto_enabled: true }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    await screen.findByText('Allow dynamic_workflow mode=auto')
    await user.click(within(dynamicToggleRow()).getByRole('switch'))

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ dynamic_workflow_auto_enabled: false })
    })
  })
})
