import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SystemAgentsSection } from './SystemAgentsSection'
import * as systemAgentDefsApi from '@/api/systemAgentDefs'
import { renderWithQuery } from '@/test/utils'
import { parseOptionalInt } from './AgentForm'
import type { SystemAgentDef } from '@/api/systemAgentDefs'

vi.mock('@/api/systemAgentDefs')

function makeAgent(overrides: Partial<SystemAgentDef> = {}): SystemAgentDef {
  return {
    id: 'conflict-resolver',
    model: 'sonnet',
    timeout: 30,
    prompt: 'Resolve merge conflicts in ${BRANCH_NAME}',
    restart_threshold: null,
    max_fail_restarts: null,
    stall_start_timeout_sec: null,
    stall_running_timeout_sec: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
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

describe('SystemAgentsSection', () => {
  beforeEach(() => vi.clearAllMocks())

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
        model: 'sonnet',
        timeout: 30,
        prompt: 'My prompt',
        restart_threshold: null,
        max_fail_restarts: null,
        stall_start_timeout_sec: null,
        stall_running_timeout_sec: null,
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
