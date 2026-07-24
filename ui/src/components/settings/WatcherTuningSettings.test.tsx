import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WatcherTuningSettings } from './WatcherTuningSettings'
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
    ...overrides,
  }
}

describe('WatcherTuningSettings', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders every knob input seeded from the provided settings values', () => {
    renderWithQuery(<WatcherTuningSettings settings={makeSettings()} />)

    expect(screen.getByPlaceholderText('0.65')).toHaveValue('0.65')
    expect(screen.getByText('Context budget default (tokens)')).toBeInTheDocument()
    expect(screen.getByText('Context decay turns')).toBeInTheDocument()
    expect(screen.getByText('Cache TTL (sec)')).toBeInTheDocument()
    expect(screen.getByText('Min epoch interval (calls)')).toBeInTheDocument()
    expect(screen.getByText('Proactive restart threshold (default)')).toBeInTheDocument()
    expect(screen.getByText('Proactive restart min interval (sec)')).toBeInTheDocument()
    expect(screen.getByText('Proactive restart max per session')).toBeInTheDocument()
    expect(screen.getByText('Proactive restart boundary window (turns)')).toBeInTheDocument()
    expect(screen.getByText('Proactive restart console (%)')).toBeInTheDocument()

    const decayInput = screen.getByText('Context decay turns')
      .closest('div.justify-between')!
      .querySelector('input') as HTMLInputElement
    expect(decayInput).toHaveValue('20')
  })

  it('renders inputs as empty when a knob value is null', () => {
    renderWithQuery(<WatcherTuningSettings settings={makeSettings({ cache_ttl_sec: null })} />)

    const cacheInput = screen.getByText('Cache TTL (sec)')
      .closest('div.justify-between')!
      .querySelector('input') as HTMLInputElement
    expect(cacheInput).toHaveValue('')
  })

  it('editing a knob and blurring calls updateGlobalSettings with exactly the changed key/value', async () => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<WatcherTuningSettings settings={makeSettings({ context_decay_turns: 20 })} />)
    const user = userEvent.setup()

    const decayInput = screen.getByText('Context decay turns')
      .closest('div.justify-between')!
      .querySelector('input') as HTMLInputElement

    await user.clear(decayInput)
    await user.type(decayInput, '30')
    await user.tab()

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ context_decay_turns: 30 })
    })
  })

  it('editing a fraction knob and pressing Enter calls updateGlobalSettings with the parsed float', async () => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<WatcherTuningSettings settings={makeSettings({ context_budget_fraction: 0.65 })} />)
    const user = userEvent.setup()

    const fractionInput = screen.getByPlaceholderText('0.65')
    await user.clear(fractionInput)
    await user.type(fractionInput, '0.8{Enter}')

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ context_budget_fraction: 0.8 })
    })
  })

  it('reverts to the prop value and does not call the mutation on an invalid (negative) entry', async () => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<WatcherTuningSettings settings={makeSettings({ cache_ttl_sec: 300 })} />)
    const user = userEvent.setup()

    const cacheInput = screen.getByText('Cache TTL (sec)')
      .closest('div.justify-between')!
      .querySelector('input') as HTMLInputElement

    await user.clear(cacheInput)
    await user.type(cacheInput, '-5')
    await user.tab()

    await waitFor(() => {
      expect(cacheInput).toHaveValue('300')
    })
    expect(settingsApi.updateGlobalSettings).not.toHaveBeenCalled()
  })

  it('does not call the mutation when blurring without changes', async () => {
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<WatcherTuningSettings settings={makeSettings({ min_epoch_interval_calls: 5 })} />)
    const user = userEvent.setup()

    const input = screen.getByText('Min epoch interval (calls)')
      .closest('div.justify-between')!
      .querySelector('input') as HTMLInputElement

    await user.click(input)
    await user.tab()

    expect(settingsApi.updateGlobalSettings).not.toHaveBeenCalled()
  })
})
