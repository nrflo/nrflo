import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { GlobalRefinerySettings } from './GlobalRefinerySettings'
import * as settingsApi from '@/api/settings'
import type { GlobalSettings } from '@/api/settings'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/api/settings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/settings')>()
  return { ...actual, updateGlobalSettings: vi.fn() }
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
    experimental_observer_enabled: false,
    observer_system_context: '',
    observer_provider: '',
    observer_model: '',
    stall_start_timeout_sec: null,
    stall_running_timeout_sec: null,
    menu_new_ticket: false,
    menu_import_spec: false,
    menu_git: false,
    menu_chain_executions: false,
    menu_schedules: false,
    menu_workflow_chains: false,
    menu_python_scripts: false,
    menu_documentation: false,
    menu_errors: false,
    menu_agent_sessions: false,
    dynamic_workflow_auto_enabled: false,
    console_yolo: true,
    context_budget_fraction: 0.65,
    context_budget_default: 32000,
    context_decay_turns: 20,
    cache_ttl_sec: 300,
    min_epoch_interval_calls: 5,
    proactive_restart_threshold_default: 80,
    proactive_restart_min_interval_sec: 60,
    proactive_restart_max_per_session: 3,
    proactive_restart_boundary_window_turns: 2,
    proactive_restart_console_pct: 90,
    refinery_fold_start_context_pct: 40,
    refinery_console_fold_start_context_pct: 75,
    refinery_fold_start_pct_premium: null,
    refinery_fold_start_pct_mid: null,
    refinery_fold_start_pct_cheap: null,
    ...overrides,
  }
}

describe('GlobalRefinerySettings', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders seeded from the settings value, and empty when null', () => {
    const { unmount } = renderWithQuery(<GlobalRefinerySettings settings={makeSettings({ refinery_fold_start_context_pct: 40 })} />)
    expect(screen.getByPlaceholderText('60')).toHaveValue('40')
    unmount()

    renderWithQuery(<GlobalRefinerySettings settings={makeSettings({ refinery_fold_start_context_pct: null })} />)
    expect(screen.getByPlaceholderText('60')).toHaveValue('')
  })

  it('typing a value and blurring calls updateGlobalSettings with exactly the parsed field', async () => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalRefinerySettings settings={makeSettings({ refinery_fold_start_context_pct: 40 })} />)
    const user = userEvent.setup()

    const input = screen.getByPlaceholderText('60')
    await user.clear(input)
    await user.type(input, '25')
    await user.tab()

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ refinery_fold_start_context_pct: 25 })
    })
  })

  it('Enter key submits the same way as blur', async () => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalRefinerySettings settings={makeSettings({ refinery_fold_start_context_pct: 40 })} />)
    const user = userEvent.setup()

    const input = screen.getByPlaceholderText('60')
    await user.clear(input)
    await user.type(input, '55{Enter}')

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ refinery_fold_start_context_pct: 55 })
    })
  })

  it.each([0, 100])('accepts boundary value %i and submits it', async (boundary) => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalRefinerySettings settings={makeSettings({ refinery_fold_start_context_pct: 40 })} />)
    const user = userEvent.setup()

    const input = screen.getByPlaceholderText('60')
    await user.clear(input)
    await user.type(input, String(boundary))
    await user.tab()

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ refinery_fold_start_context_pct: boundary })
    })
  })

  it.each([101, -1])('reverts to the server value and does not call the mutation for out-of-range %i', async (bad) => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalRefinerySettings settings={makeSettings({ refinery_fold_start_context_pct: 40 })} />)
    const user = userEvent.setup()

    const input = screen.getByPlaceholderText('60')
    await user.clear(input)
    await user.type(input, String(bad))
    await user.tab()

    await waitFor(() => {
      expect(input).toHaveValue('40')
    })
    expect(settingsApi.updateGlobalSettings).not.toHaveBeenCalled()
  })

  it('clearing the input submits null', async () => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalRefinerySettings settings={makeSettings({ refinery_fold_start_context_pct: 40 })} />)
    const user = userEvent.setup()

    const input = screen.getByPlaceholderText('60')
    await user.clear(input)
    await user.tab()

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ refinery_fold_start_context_pct: null })
    })
  })

  it('blurring without changing the value does not call the mutation', async () => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalRefinerySettings settings={makeSettings({ refinery_fold_start_context_pct: 40 })} />)
    const user = userEvent.setup()

    const input = screen.getByPlaceholderText('60')
    await user.click(input)
    await user.tab()

    expect(settingsApi.updateGlobalSettings).not.toHaveBeenCalled()
  })

  it('console fold-start row submits its own key', async () => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalRefinerySettings settings={makeSettings()} />)
    const user = userEvent.setup()

    const input = screen.getByPlaceholderText('75')
    await user.clear(input)
    await user.type(input, '80')
    await user.tab()

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ refinery_console_fold_start_context_pct: 80 })
    })
  })

  it('non-numeric input parses to null (per parseOptionalInt) and submits null', async () => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalRefinerySettings settings={makeSettings({ refinery_fold_start_context_pct: 40 })} />)
    const user = userEvent.setup()

    const input = screen.getByPlaceholderText('60')
    await user.clear(input)
    await user.type(input, 'abc')
    await user.tab()

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ refinery_fold_start_context_pct: null })
    })
  })
})
