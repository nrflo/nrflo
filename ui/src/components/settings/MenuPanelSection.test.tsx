import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MenuPanelSection } from './MenuPanelSection'
import * as settingsApi from '@/api/settings'
import type { GlobalSettings } from '@/api/settings'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/api/settings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/settings')>()
  return { ...actual, getGlobalSettings: vi.fn(), updateGlobalSettings: vi.fn() }
})

function makeSettings(overrides: Partial<GlobalSettings> = {}): GlobalSettings {
  return {
    menu_new_ticket: false,
    menu_import_spec: false,
    menu_git: true,
    menu_chain_executions: true,
    menu_schedules: false,
    menu_workflow_chains: false,
    menu_python_scripts: false,
    menu_documentation: true,
    menu_errors: false,
    menu_agent_sessions: false,
    ...overrides,
  } as GlobalSettings
}

describe('MenuPanelSection', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders all 10 toggle rows with correct labels in documented order', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    renderWithQuery(<MenuPanelSection />)

    const labels = [
      'New Ticket', 'Import Spec', 'Git', 'Chain Executions', 'Schedules',
      'Workflow Chains', 'Python Scripts', 'Documentation', 'Errors', 'Agent Sessions',
    ]
    for (const label of labels) {
      expect(await screen.findByText(label)).toBeInTheDocument()
    }
    expect(screen.getAllByRole('switch')).toHaveLength(10)
  })

  it('reflects documented defaults: git/chain_executions/documentation=true, rest=false', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    renderWithQuery(<MenuPanelSection />)

    // Order: new_ticket, import_spec, git, chain_executions, schedules,
    //        workflow_chains, python_scripts, documentation, errors, agent_sessions
    const expected = [false, false, true, true, false, false, false, true, false, false]
    const toggles = await screen.findAllByRole('switch')
    expected.forEach((checked, i) => {
      expect(toggles[i]).toHaveAttribute('aria-checked', String(checked))
    })
  })

  it('clicking a toggle calls updateGlobalSettings once with the matching key and toggled value', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<MenuPanelSection />)

    const user = userEvent.setup()
    // toggles[0] = New Ticket (default false → click → true)
    await user.click((await screen.findAllByRole('switch'))[0])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledTimes(1)
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ menu_new_ticket: true })
    })
  })

  it('clicking git toggle (default true) sends false', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    vi.mocked(settingsApi.updateGlobalSettings).mockResolvedValue(undefined)
    renderWithQuery(<MenuPanelSection />)

    const user = userEvent.setup()
    // toggles[2] = Git (default true → click → false)
    await user.click((await screen.findAllByRole('switch'))[2])

    await waitFor(() => {
      expect(settingsApi.updateGlobalSettings).toHaveBeenCalledWith({ menu_git: false })
    })
  })

  it('pending mutation disables all toggles', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockResolvedValue(makeSettings())
    vi.mocked(settingsApi.updateGlobalSettings).mockReturnValue(new Promise(() => {}))
    renderWithQuery(<MenuPanelSection />)

    const user = userEvent.setup()
    await user.click((await screen.findAllByRole('switch'))[0])

    await waitFor(() => {
      screen.getAllByRole('switch').forEach((t) => expect(t).toBeDisabled())
    })
  })

  it('shows loading state while fetching', () => {
    vi.mocked(settingsApi.getGlobalSettings).mockReturnValue(new Promise(() => {}))
    renderWithQuery(<MenuPanelSection />)
    expect(screen.getByText('Loading settings...')).toBeInTheDocument()
  })

  it('shows error when fetch fails', async () => {
    vi.mocked(settingsApi.getGlobalSettings).mockRejectedValue(new Error('Network error'))
    renderWithQuery(<MenuPanelSection />)
    expect(await screen.findByText(/Error: Network error/)).toBeInTheDocument()
  })
})
