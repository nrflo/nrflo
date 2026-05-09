import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProjectEnvVarsEditor } from './ProjectEnvVarsEditor'
import * as api from '@/api/projectEnvVars'
import { renderWithQuery } from '@/test/utils'
import type { ProjectEnvVar } from '@/api/projectEnvVars'

vi.mock('@/api/projectEnvVars')

function makeVar(overrides: Partial<ProjectEnvVar> = {}): ProjectEnvVar {
  return {
    name: 'API_KEY',
    value: 'secret',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const PROJECT_ID = 'proj-1'

beforeEach(() => vi.clearAllMocks())

describe('ProjectEnvVarsEditor', () => {
  it('renders existing var rows with name and value', async () => {
    vi.mocked(api.listEnvVars).mockResolvedValue([
      makeVar({ name: 'API_KEY', value: 'abc123' }),
      makeVar({ name: 'DB_URL', value: 'postgres://localhost' }),
    ])

    renderWithQuery(<ProjectEnvVarsEditor projectId={PROJECT_ID} />)

    expect(await screen.findByText('API_KEY')).toBeInTheDocument()
    expect(screen.getByText('abc123')).toBeInTheDocument()
    expect(screen.getByText('DB_URL')).toBeInTheDocument()
    expect(screen.getByText('postgres://localhost')).toBeInTheDocument()
  })

  it('shows loading spinner then hides it after load', async () => {
    vi.mocked(api.listEnvVars).mockResolvedValue([])

    renderWithQuery(<ProjectEnvVarsEditor projectId={PROJECT_ID} />)

    // After load, spinner is gone and add row inputs are visible
    expect(await screen.findByPlaceholderText('VAR_NAME')).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('add flow: typing name + value and clicking Plus calls putEnvVar and shows new row', async () => {
    vi.mocked(api.listEnvVars)
      .mockResolvedValueOnce([])
      .mockResolvedValue([makeVar({ name: 'NEW_VAR', value: 'newval' })])
    vi.mocked(api.putEnvVar).mockResolvedValue(makeVar({ name: 'NEW_VAR', value: 'newval' }))

    renderWithQuery(<ProjectEnvVarsEditor projectId={PROJECT_ID} />)
    await screen.findByPlaceholderText('VAR_NAME')

    const user = userEvent.setup()
    await user.type(screen.getByPlaceholderText('VAR_NAME'), 'NEW_VAR')
    await user.type(screen.getByPlaceholderText('value'), 'newval')

    // Only one button exists (Plus) when there are no existing vars
    await user.click(screen.getByRole('button'))

    await waitFor(() =>
      expect(api.putEnvVar).toHaveBeenCalledWith(PROJECT_ID, 'NEW_VAR', 'newval')
    )
    expect(await screen.findByText('NEW_VAR')).toBeInTheDocument()
  })

  it('edit flow: clicking edit, changing value, clicking save calls putEnvVar with original name', async () => {
    vi.mocked(api.listEnvVars)
      .mockResolvedValueOnce([makeVar({ name: 'API_KEY', value: 'old' })])
      .mockResolvedValue([makeVar({ name: 'API_KEY', value: 'new' })])
    vi.mocked(api.putEnvVar).mockResolvedValue(makeVar({ name: 'API_KEY', value: 'new' }))

    renderWithQuery(<ProjectEnvVarsEditor projectId={PROJECT_ID} />)
    await screen.findByText('API_KEY')

    const user = userEvent.setup()
    // Buttons in non-edit state: [Pencil(0), Trash2(1), Plus(2)]
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[0]) // Pencil → startEdit

    // Edit input appears pre-filled with current value
    const editInput = screen.getByDisplayValue('old')
    await user.clear(editInput)
    await user.type(editInput, 'new')

    // Buttons in edit state: [Check(0), X(1), Plus(2)]
    await user.click(screen.getAllByRole('button')[0]) // Check → saveEdit

    await waitFor(() =>
      expect(api.putEnvVar).toHaveBeenCalledWith(PROJECT_ID, 'API_KEY', 'new')
    )
  })

  it('delete flow: clicking delete opens confirm dialog, confirming calls deleteEnvVar', async () => {
    vi.mocked(api.listEnvVars)
      .mockResolvedValueOnce([makeVar({ name: 'OLD_KEY', value: 'val' })])
      .mockResolvedValue([])
    vi.mocked(api.deleteEnvVar).mockResolvedValue(undefined)

    renderWithQuery(<ProjectEnvVarsEditor projectId={PROJECT_ID} />)
    await screen.findByText('OLD_KEY')

    const user = userEvent.setup()
    // Buttons: [Pencil(0), Trash2(1), Plus(2)]
    await user.click(screen.getAllByRole('button')[1]) // Trash2 → open ConfirmDialog

    expect(screen.getByText('Delete variable')).toBeInTheDocument()
    expect(screen.getByText(/Delete environment variable "OLD_KEY"/)).toBeInTheDocument()

    // ConfirmDialog has Cancel + Delete buttons; click Delete
    await user.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() =>
      expect(api.deleteEnvVar).toHaveBeenCalledWith(PROJECT_ID, 'OLD_KEY')
    )
  })

  it('error path: putEnvVar rejection shows error message inline', async () => {
    vi.mocked(api.listEnvVars).mockResolvedValue([])
    vi.mocked(api.putEnvVar).mockRejectedValue(new Error('name NRF_SESSION_ID is reserved'))

    renderWithQuery(<ProjectEnvVarsEditor projectId={PROJECT_ID} />)
    await screen.findByPlaceholderText('VAR_NAME')

    const user = userEvent.setup()
    await user.type(screen.getByPlaceholderText('VAR_NAME'), 'NRF_SESSION_ID')
    await user.type(screen.getByPlaceholderText('value'), 'x')
    await user.click(screen.getByRole('button'))

    expect(await screen.findByText('name NRF_SESSION_ID is reserved')).toBeInTheDocument()
  })
})
