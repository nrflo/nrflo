import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SystemAgentsSection } from './SystemAgentsSection'
import * as systemAgentDefsApi from '@/api/systemAgentDefs'
import * as tierModelsApi from '@/api/tierModels'
import { renderWithQuery } from '@/test/utils'
import { parseOptionalInt } from './AgentForm'
import type { SystemAgentDef } from '@/api/systemAgentDefs'

vi.mock('@/api/systemAgentDefs')
vi.mock('@/api/tierModels')

function makeAgent(overrides: Partial<SystemAgentDef> = {}): SystemAgentDef {
  return {
    id: 'conflict-resolver',
    model: 'sonnet-5',
    execution_mode: 'cli_interactive',
    timeout: 30,
    prompt: 'Resolve merge conflicts in ${BRANCH_NAME}',
    restart_threshold: null,
    max_fail_restarts: null,
    stall_start_timeout_sec: null,
    stall_running_timeout_sec: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    tier: 1,
    reasoning_effort: null,
    ...overrides,
  }
}

function makeTierRow(overrides: Partial<tierModelsApi.TierModel> = {}): tierModelsApi.TierModel {
  return {
    tier: 1,
    position: 0,
    provider: 'anthropic',
    execution_mode: 'cli_interactive',
    model_id: 'tier-1-primary',
    reasoning_effort: '',
    ...overrides,
  }
}

describe('parseOptionalInt', () => {
  it('returns null for empty or whitespace string', () => {
    expect(parseOptionalInt('')).toBeNull()
    expect(parseOptionalInt('  ')).toBeNull()
  })

  it('parses valid number string', () => {
    expect(parseOptionalInt('25')).toBe(25)
  })

  it('returns null for non-numeric string', () => {
    expect(parseOptionalInt('abc')).toBeNull()
  })
})

describe('SystemAgentsSection — warning banner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(tierModelsApi.listTierModels).mockResolvedValue([])
  })

  it('shows lead text in empty-list state', async () => {
    vi.mocked(systemAgentDefsApi.listSystemAgentDefs).mockResolvedValue([])
    renderWithQuery(<SystemAgentsSection />)
    expect(
      await screen.findByText(/Mode determines how a system agent runs/i)
    ).toBeInTheDocument()
  })

  it('shows lead text when agents are listed', async () => {
    vi.mocked(systemAgentDefsApi.listSystemAgentDefs).mockResolvedValue([makeAgent()])
    renderWithQuery(<SystemAgentsSection />)
    await screen.findByText('conflict-resolver')
    expect(
      screen.getByText(/Mode determines how a system agent runs/i)
    ).toBeInTheDocument()
  })

  it('has no dismiss or close button', async () => {
    vi.mocked(systemAgentDefsApi.listSystemAgentDefs).mockResolvedValue([])
    renderWithQuery(<SystemAgentsSection />)
    await screen.findByText(/Mode determines how a system agent runs/i)
    // Only the "New System Agent" button should exist at this point
    const buttons = screen.getAllByRole('button')
    expect(buttons).toHaveLength(1)
    expect(buttons[0]).toHaveTextContent(/New System Agent/)
  })
})

describe('SystemAgentsSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(tierModelsApi.listTierModels).mockResolvedValue([])
  })

  it('shows empty state when no agents and error state on failure', async () => {
    vi.mocked(systemAgentDefsApi.listSystemAgentDefs).mockResolvedValue([])
    const { unmount } = renderWithQuery(<SystemAgentsSection />)
    expect(
      await screen.findByText('No system agents defined. Create one to get started.')
    ).toBeInTheDocument()
    unmount()

    vi.mocked(systemAgentDefsApi.listSystemAgentDefs).mockRejectedValue(new Error('Server error'))
    renderWithQuery(<SystemAgentsSection />)
    expect(await screen.findByText(/Error: Server error/)).toBeInTheDocument()
  })

  it('create form: opens, validates required fields, submits with null optional fields, cancels', async () => {
    vi.mocked(systemAgentDefsApi.listSystemAgentDefs)
      .mockResolvedValueOnce([])
      .mockResolvedValue([makeAgent({ id: 'my-agent' })])
    vi.mocked(systemAgentDefsApi.createSystemAgentDef).mockResolvedValue(
      makeAgent({ id: 'my-agent' })
    )

    renderWithQuery(<SystemAgentsSection />)
    await screen.findByText('No system agents defined. Create one to get started.')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /New System Agent/ }))
    expect(screen.getByPlaceholderText('conflict-resolver')).toBeInTheDocument()

    // Save disabled until both required fields filled
    const createBtn = screen.getByRole('button', { name: 'Create' })
    expect(createBtn).toBeDisabled()
    await user.type(screen.getByPlaceholderText('conflict-resolver'), 'my-agent')
    expect(createBtn).toBeDisabled()
    await user.type(screen.getByPlaceholderText('Agent prompt template...'), 'My prompt')
    expect(createBtn).not.toBeDisabled()

    // Submit — verifies null for empty optional numeric fields
    await user.click(createBtn)
    await waitFor(() => {
      expect(systemAgentDefsApi.createSystemAgentDef).toHaveBeenCalledWith({
        id: 'my-agent',
        model: '',
        execution_mode: 'cli_interactive',
        timeout: 30,
        prompt: 'My prompt',
        restart_threshold: null,
        max_fail_restarts: null,
        stall_start_timeout_sec: null,
        stall_running_timeout_sec: null,
        tier: 1,
        reasoning_effort: null,
      })
    })
  })

  it('read row shows Tier N badge and resolves the primary model from the tier chain when there is no override', async () => {
    vi.mocked(systemAgentDefsApi.listSystemAgentDefs).mockResolvedValue([
      makeAgent({ id: 'no-override-agent', model: '', tier: 2 }),
    ])
    vi.mocked(tierModelsApi.listTierModels).mockResolvedValue([
      makeTierRow({ tier: 2, position: 0, model_id: 'tier-2-primary' }),
      makeTierRow({ tier: 2, position: 1, model_id: 'tier-2-fallback' }),
    ])
    renderWithQuery(<SystemAgentsSection />)

    await screen.findByText('no-override-agent')
    expect(screen.getByText('Tier 2')).toBeInTheDocument()
    expect(screen.queryByText('Override')).not.toBeInTheDocument()
    expect(screen.getByText(/Model: tier-2-primary/)).toBeInTheDocument()
  })

  it('an override wins over the tier chain in both display and the update payload', async () => {
    vi.mocked(systemAgentDefsApi.listSystemAgentDefs)
      .mockResolvedValueOnce([
        makeAgent({ id: 'override-agent', model: 'opus', tier: 3, reasoning_effort: 'high' }),
      ])
      .mockResolvedValue([])
    vi.mocked(tierModelsApi.listTierModels).mockResolvedValue([
      makeTierRow({ tier: 3, position: 0, model_id: 'tier-3-primary' }),
    ])
    vi.mocked(systemAgentDefsApi.updateSystemAgentDef).mockResolvedValue({ status: 'ok' })

    renderWithQuery(<SystemAgentsSection />)
    await screen.findByText('override-agent')

    // Display: override badge present, model shown is the override, not the tier chain's primary
    expect(screen.getByText('Tier 3')).toBeInTheDocument()
    expect(screen.getByText('Override')).toBeInTheDocument()
    expect(screen.getByText(/Model: opus/)).toBeInTheDocument()
    expect(screen.queryByText(/tier-3-primary/)).not.toBeInTheDocument()

    // Editing: no override toggle available before opening — open the edit form
    const user = userEvent.setup()
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[1]) // pencil (edit)
    await screen.findByDisplayValue('override-agent')

    // Override is pre-populated on because the agent has a model — no raw Model dropdown behind a disabled toggle
    expect(screen.getByRole('switch', { name: /Override model/ })).toHaveAttribute('aria-checked', 'true')

    await user.click(screen.getByRole('button', { name: /Save/ }))
    await waitFor(() => {
      expect(systemAgentDefsApi.updateSystemAgentDef).toHaveBeenCalledWith('override-agent', {
        model: 'opus',
        execution_mode: 'cli_interactive',
        timeout: 30,
        prompt: 'Resolve merge conflicts in ${BRANCH_NAME}',
        restart_threshold: null,
        max_fail_restarts: null,
        stall_start_timeout_sec: null,
        stall_running_timeout_sec: null,
        tier: 3,
        reasoning_effort: 'high',
      })
    })
  })

  it('agent list display, edit form pre-population, and delete confirmation flow', async () => {
    vi.mocked(systemAgentDefsApi.listSystemAgentDefs)
      .mockResolvedValueOnce([
        makeAgent({ id: 'conflict-resolver', model: 'opus', timeout: 60, restart_threshold: 25 }),
      ])
      .mockResolvedValue([])
    vi.mocked(systemAgentDefsApi.deleteSystemAgentDef).mockResolvedValue({ status: 'ok' })

    renderWithQuery(<SystemAgentsSection />)
    await screen.findByText('conflict-resolver')

    // Display: shows model and timeout
    expect(screen.getByText(/Model: opus/)).toBeInTheDocument()
    expect(screen.getByText(/Timeout: 60m/)).toBeInTheDocument()

    const user = userEvent.setup()

    // Edit: buttons[0]=New System Agent, buttons[1]=pencil(edit), buttons[2]=trash(delete)
    let buttons = screen.getAllByRole('button')
    await user.click(buttons[1])

    // Edit form shows pre-populated data with ID disabled
    const idInput = await screen.findByDisplayValue('conflict-resolver')
    expect(idInput).toBeDisabled()
    expect(screen.getByDisplayValue('25')).toBeInTheDocument() // restart_threshold
    expect(screen.getByDisplayValue('60')).toBeInTheDocument() // timeout

    // Cancel edit returns to display mode
    await user.click(screen.getByRole('button', { name: /Cancel/ }))
    expect(await screen.findByText('conflict-resolver')).toBeInTheDocument()

    // Delete: show confirmation, cancel dismisses it
    buttons = screen.getAllByRole('button')
    await user.click(buttons[2]) // trash button
    expect(screen.getByText(/Are you sure you want to delete/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByText(/Are you sure you want to delete/)).not.toBeInTheDocument()

    // Delete: confirm deletes the agent
    buttons = screen.getAllByRole('button')
    await user.click(buttons[2])
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => {
      expect(systemAgentDefsApi.deleteSystemAgentDef).toHaveBeenCalledWith('conflict-resolver')
    })
  })
})
