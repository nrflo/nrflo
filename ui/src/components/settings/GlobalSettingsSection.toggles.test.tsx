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
    stall_start_timeout_sec: null,
    stall_running_timeout_sec: null,
    ...overrides,
  }
}

// Toggle DOM order: [0]=api_mode, [1]=api_native_tools, [2]=system_prompt_override, [3]=low_consumption,
// [4]=simplified_graph, [5]=experimental, [6]=api_via_cli, [7]=observer, [8]=capture_thinking, [9]=dynamic_workflow_auto.
describe('GlobalSettingsSection boolean toggles', () => {
  beforeEach(() => vi.clearAllMocks())

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
    expect(toggles[5]).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByText('Experimental features')).toBeInTheDocument()
  })

  it('renders Experimental features toggle reflecting server state (true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ experimental: true }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[5]).toHaveAttribute('aria-checked', 'true')
  })

  it('clicking Experimental toggle calls updateGlobalSettings({ experimental: true })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ experimental: false }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[5])

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
    await user.click(toggles[5])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ experimental: false })
    })
  })

  it('renders api_via_cli_enabled toggle reflecting server state (false)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ api_via_cli_enabled: false }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[6]).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByText('Route API agents via Claude CLI')).toBeInTheDocument()
  })

  it('renders api_via_cli_enabled toggle reflecting server state (true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ api_via_cli_enabled: true }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[6]).toHaveAttribute('aria-checked', 'true')
  })

  it('clicking api_via_cli_enabled toggle (false→true) calls updateGlobalSettings({ api_via_cli_enabled: true })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ api_via_cli_enabled: false }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[6])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ api_via_cli_enabled: true })
    })
  })

  it('clicking api_via_cli_enabled toggle (true→false) calls updateGlobalSettings({ api_via_cli_enabled: false })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ api_via_cli_enabled: true }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[6])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ api_via_cli_enabled: false })
    })
  })

  it('renders Override Claude system prompt toggle reflecting server state (false)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ claude_system_prompt_override_enabled: false }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[2]).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByText('Override Claude system prompt')).toBeInTheDocument()
    expect(screen.getByText(/Replaces the default Claude Code system prompt/i)).toBeInTheDocument()
  })

  it('renders Override Claude system prompt toggle reflecting server state (true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ claude_system_prompt_override_enabled: true }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[2]).toHaveAttribute('aria-checked', 'true')
  })

  it('clicking Override Claude system prompt toggle (false→true) calls updateGlobalSettings({ claude_system_prompt_override_enabled: true })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ claude_system_prompt_override_enabled: false }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[2])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ claude_system_prompt_override_enabled: true })
    })
  })

  it('clicking Override Claude system prompt toggle (true→false) calls updateGlobalSettings({ claude_system_prompt_override_enabled: false })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ claude_system_prompt_override_enabled: true }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[2])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ claude_system_prompt_override_enabled: false })
    })
  })

  it('renders capture_thinking_enabled toggle reflecting server state (false)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ capture_thinking_enabled: false }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[8]).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByText('Capture model thinking')).toBeInTheDocument()
  })

  it('renders capture_thinking_enabled toggle reflecting server state (true)', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ capture_thinking_enabled: true }))
    renderWithQuery(<GlobalSettingsSection />)
    const toggles = await screen.findAllByRole('switch')
    expect(toggles[8]).toHaveAttribute('aria-checked', 'true')
  })

  it('clicking capture_thinking_enabled toggle (false→true) calls updateGlobalSettings({ capture_thinking_enabled: true })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ capture_thinking_enabled: false }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[8])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ capture_thinking_enabled: true })
    })
  })

  it('clicking capture_thinking_enabled toggle (true→false) calls updateGlobalSettings({ capture_thinking_enabled: false })', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings({ capture_thinking_enabled: true }))
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<GlobalSettingsSection />)

    const user = userEvent.setup()
    const toggles = await screen.findAllByRole('switch')
    await user.click(toggles[8])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ capture_thinking_enabled: false })
    })
  })
})
