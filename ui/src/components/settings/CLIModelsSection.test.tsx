import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CLIModelsSection } from './CLIModelsSection'
import * as cliModelsApi from '@/api/cliModels'
import { ApiError } from '@/api/client'
import { renderWithQuery } from '@/test/utils'
import type { CLIModel } from '@/api/cliModels'

vi.mock('@/api/cliModels')

function makeCLIModel(overrides: Partial<CLIModel> = {}): CLIModel {
  return {
    id: 'sonnet',
    cli_type: 'claude',
    display_name: 'Sonnet',
    mapped_model: 'claude-sonnet-4-20250514',
    reasoning_effort: '',
    context_length: 200000,
    read_only: false,
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('CLIModelsSection', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows empty state when no models', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([])
    renderWithQuery(<CLIModelsSection />)
    expect(
      await screen.findByText('No CLI models found. Create one to get started.')
    ).toBeInTheDocument()
  })

  it('shows error state on fetch failure', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockRejectedValue(new Error('fetch failed'))
    renderWithQuery(<CLIModelsSection />)
    expect(await screen.findByText(/Error: fetch failed/)).toBeInTheDocument()
  })

  it('displays model list with id, cli_type badge, display_name, mapped_model', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'opus', cli_type: 'claude', display_name: 'Opus', mapped_model: 'claude-opus-4' }),
    ])
    renderWithQuery(<CLIModelsSection />)
    expect(await screen.findByText('opus')).toBeInTheDocument()
    expect(screen.getByText('claude')).toBeInTheDocument()
    expect(screen.getByText(/Opus/)).toBeInTheDocument()
    expect(screen.getByText(/claude-opus-4/)).toBeInTheDocument()
  })

  it('shows Built-in badge for read_only models and hides edit/delete buttons', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'opus', read_only: true }),
    ])
    renderWithQuery(<CLIModelsSection />)
    expect(await screen.findByText('Built-in')).toBeInTheDocument()
    // "New Model" + Check = 2 buttons (no edit or delete)
    expect(screen.getAllByRole('button')).toHaveLength(2)
  })

  it('shows edit and delete buttons for non-readonly models', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'my-model', read_only: false }),
    ])
    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('my-model')
    // "New Model" + Check + edit pencil + trash = 4 buttons
    expect(screen.getAllByRole('button')).toHaveLength(4)
  })

  it('create form: opens, validates required fields, submits correct request', async () => {
    vi.mocked(cliModelsApi.listCLIModels)
      .mockResolvedValueOnce([])
      .mockResolvedValue([makeCLIModel({ id: 'my-model' })])
    vi.mocked(cliModelsApi.createCLIModel).mockResolvedValue(makeCLIModel({ id: 'my-model' }))

    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('No CLI models found. Create one to get started.')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /New Model/ }))

    const createBtn = screen.getByRole('button', { name: 'Create' })
    expect(createBtn).toBeDisabled()

    await user.type(screen.getByPlaceholderText('my-custom-model'), 'my-model')
    expect(createBtn).toBeDisabled()

    await user.type(screen.getByPlaceholderText('My Model'), 'My Custom Model')
    expect(createBtn).toBeDisabled()

    await user.type(screen.getByPlaceholderText('claude-sonnet-4-20250514'), 'claude-custom-v1')
    expect(createBtn).not.toBeDisabled()

    await user.click(createBtn)
    await waitFor(() => {
      expect(cliModelsApi.createCLIModel).toHaveBeenCalledWith({
        id: 'my-model',
        cli_type: 'claude',
        display_name: 'My Custom Model',
        mapped_model: 'claude-custom-v1',
        reasoning_effort: undefined,
        context_length: 200000,
      })
    })
  })

  it('cancel on create form closes without API call', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([])

    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('No CLI models found. Create one to get started.')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /New Model/ }))
    expect(screen.getByRole('button', { name: 'Create' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('button', { name: 'Create' })).not.toBeInTheDocument()
    expect(cliModelsApi.createCLIModel).not.toHaveBeenCalled()
  })

  it('New Model button is disabled while create form is open', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([])

    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('No CLI models found. Create one to get started.')

    const user = userEvent.setup()
    const newBtn = screen.getByRole('button', { name: /New Model/ })
    await user.click(newBtn)
    expect(newBtn).toBeDisabled()
  })

  it('shows amber warning when codex cli_type selected in create form', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([])

    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('No CLI models found. Create one to get started.')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /New Model/ }))

    // Open CLI type dropdown (defaults to "Claude")
    await user.click(screen.getByRole('button', { name: 'Claude' }))
    await user.click(screen.getByText('Codex'))

    expect(
      screen.getByText('Codex agents run in read-only sandboxed environments')
    ).toBeInTheDocument()
  })

  it('edit form: pre-populates fields, id disabled, saves with correct args', async () => {
    vi.mocked(cliModelsApi.listCLIModels)
      .mockResolvedValueOnce([
        makeCLIModel({ id: 'my-model', display_name: 'My Model', mapped_model: 'custom-v1', read_only: false }),
      ])
      .mockResolvedValue([])
    vi.mocked(cliModelsApi.updateCLIModel).mockResolvedValue({ status: 'ok' })

    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('my-model')

    const user = userEvent.setup()
    // buttons: [0]="New Model", [1]=check, [2]=edit pencil, [3]=trash
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[2])

    const idInput = screen.getByDisplayValue('my-model')
    expect(idInput).toBeDisabled()

    await user.click(screen.getByRole('button', { name: /Save/ }))
    await waitFor(() => {
      expect(cliModelsApi.updateCLIModel).toHaveBeenCalledWith('my-model', {
        display_name: 'My Model',
        mapped_model: 'custom-v1',
        reasoning_effort: undefined,
        context_length: 200000,
      })
    })
  })

  it('renders group headers for known cli_types', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'opus', cli_type: 'claude' }),
      makeCLIModel({ id: 'gpt', cli_type: 'codex' }),
      makeCLIModel({ id: 'oc', cli_type: 'opencode' }),
    ])
    renderWithQuery(<CLIModelsSection />)
    expect(await screen.findByText('Claude')).toBeInTheDocument()
    expect(screen.getByText('Codex')).toBeInTheDocument()
    expect(screen.getByText('OpenCode')).toBeInTheDocument()
  })

  it('does not render headers for empty groups', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'opus', cli_type: 'claude' }),
    ])
    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('Claude')
    expect(screen.queryByText('Codex')).not.toBeInTheDocument()
    expect(screen.queryByText('OpenCode')).not.toBeInTheDocument()
  })

  it('groups models with unknown cli_type under Other', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'weird-model', cli_type: 'custom-provider' }),
    ])
    renderWithQuery(<CLIModelsSection />)
    expect(await screen.findByText('Other')).toBeInTheDocument()
    expect(screen.getByText('weird-model')).toBeInTheDocument()
    expect(screen.queryByText('Claude')).not.toBeInTheDocument()
  })

  it('shows timeout tooltip for OpenCode group header only', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'claude-model', cli_type: 'claude' }),
      makeCLIModel({ id: 'codex-model', cli_type: 'codex' }),
      makeCLIModel({ id: 'oc-model', cli_type: 'opencode' }),
    ])
    renderWithQuery(<CLIModelsSection />)
    const user = userEvent.setup()

    // OpenCode heading has Info icon (SVG) → tooltip on hover
    const openCodeHeading = await screen.findByText('OpenCode')
    const infoSvg = openCodeHeading.querySelector('svg')
    expect(infoSvg).toBeInTheDocument()
    await user.hover(infoSvg!)
    expect(await screen.findByRole('tooltip')).toHaveTextContent('OpenAI models will timeout on failure')

    // Claude and Codex headings have no Info icon
    expect(screen.getByText('Claude').querySelector('svg')).not.toBeInTheDocument()
    expect(screen.getByText('Codex').querySelector('svg')).not.toBeInTheDocument()
  })

  it('groups appear in correct order: Claude before Codex before OpenCode', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'oc-model', cli_type: 'opencode' }),
      makeCLIModel({ id: 'codex-model', cli_type: 'codex' }),
      makeCLIModel({ id: 'claude-model', cli_type: 'claude' }),
    ])
    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('Claude')
    const headers = screen.getAllByRole('heading')
    const headerTexts = headers.map((h) => h.textContent)
    expect(headerTexts.indexOf('Claude')).toBeLessThan(headerTexts.indexOf('Codex'))
    expect(headerTexts.indexOf('Codex')).toBeLessThan(headerTexts.indexOf('OpenCode'))
  })

  it('toggle renders checked and disabled for read_only models', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'opus', read_only: true, enabled: true }),
    ])
    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('opus')
    const toggle = screen.getByRole('switch')
    expect(toggle).toBeChecked()
    expect(toggle).toBeDisabled()
  })

  it('toggle calls updateCLIModel with enabled:false when disabling a custom model', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'my-model', read_only: false, enabled: true }),
    ])
    vi.mocked(cliModelsApi.updateCLIModel).mockResolvedValue({ status: 'ok' })

    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('my-model')

    const user = userEvent.setup()
    await user.click(screen.getByRole('switch'))

    await waitFor(() => {
      expect(cliModelsApi.updateCLIModel).toHaveBeenCalledWith('my-model', { enabled: false })
    })
  })

  it('toggle calls updateCLIModel with enabled:true when enabling a disabled custom model', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'my-model', read_only: false, enabled: false }),
    ])
    vi.mocked(cliModelsApi.updateCLIModel).mockResolvedValue({ status: 'ok' })

    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('my-model')

    const user = userEvent.setup()
    await user.click(screen.getByRole('switch'))

    await waitFor(() => {
      expect(cliModelsApi.updateCLIModel).toHaveBeenCalledWith('my-model', { enabled: true })
    })
  })

  it('displays inline error when toggle API returns 409', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'my-model', read_only: false, enabled: true }),
    ])
    vi.mocked(cliModelsApi.updateCLIModel).mockRejectedValue(
      new ApiError(409, 'model is in use by: myproject/feature/implementor')
    )

    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('my-model')

    const user = userEvent.setup()
    await user.click(screen.getByRole('switch'))

    expect(
      await screen.findByText('model is in use by: myproject/feature/implementor')
    ).toBeInTheDocument()
  })

  it('delete: confirmation dialog, cancel dismisses, confirm calls API', async () => {
    vi.mocked(cliModelsApi.listCLIModels)
      .mockResolvedValueOnce([makeCLIModel({ id: 'custom-model', read_only: false })])
      .mockResolvedValue([])
    vi.mocked(cliModelsApi.deleteCLIModel).mockResolvedValue({ status: 'ok' })

    renderWithQuery(<CLIModelsSection />)
    await screen.findByText('custom-model')

    const user = userEvent.setup()
    let buttons = screen.getAllByRole('button')
    await user.click(buttons[3]) // trash

    expect(screen.getByText(/Are you sure you want to delete/)).toBeInTheDocument()

    // Cancel dismisses without deleting
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByText(/Are you sure you want to delete/)).not.toBeInTheDocument()
    expect(cliModelsApi.deleteCLIModel).not.toHaveBeenCalled()

    // Re-open and confirm
    buttons = screen.getAllByRole('button')
    await user.click(buttons[3])
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => {
      expect(cliModelsApi.deleteCLIModel).toHaveBeenCalledWith('custom-model')
    })
  })
})
