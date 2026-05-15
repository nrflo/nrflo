import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/utils'
import { ProviderModelsList } from './ProviderModelsList'
import type { CLIModel } from '@/api/cliModels'

vi.mock('@/hooks/useCLIModels', () => ({
  useCLIModels: vi.fn(),
  cliModelKeys: { list: () => ['cli-models', 'list'] },
}))

vi.mock('@/api/cliModels', () => ({
  createCLIModel: vi.fn(),
  updateCLIModel: vi.fn(),
  deleteCLIModel: vi.fn(),
}))

vi.mock('./CLIModelCheckButton', () => ({
  CLIModelCheckButton: () => <button>Check</button>,
}))

vi.mock('./CLIModelForm', () => ({
  CLIModelForm: ({
    onCancel,
    onSave,
    isCreate,
  }: {
    onCancel: () => void
    onSave: () => void
    isCreate?: boolean
    [k: string]: unknown
  }) => (
    <div data-testid="cli-model-form">
      {isCreate && <span>create-form</span>}
      <button onClick={onCancel}>Cancel</button>
      <button onClick={onSave}>Save</button>
    </div>
  ),
  emptyCLIModelForm: {
    id: '',
    cli_type: 'claude',
    display_name: '',
    mapped_model: '',
    reasoning_effort: '',
    context_length: '200000',
  },
}))

import { useCLIModels } from '@/hooks/useCLIModels'
import * as cliModelsApi from '@/api/cliModels'

function makeModel(overrides: Partial<CLIModel> = {}): CLIModel {
  return {
    id: 'sonnet-4-6',
    cli_type: 'claude',
    display_name: 'Sonnet 4.6',
    mapped_model: 'claude-sonnet-4-6',
    reasoning_effort: '',
    context_length: 200000,
    read_only: false,
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderList(provider: 'claude' | 'opencode' | 'codex' | 'gemini' = 'claude') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <ProviderModelsList provider={provider} />
    </QueryClientProvider>
  )
}

describe('ProviderModelsList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useCLIModels).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useCLIModels>)
  })

  it('shows empty state when no models for provider', () => {
    renderList('claude')
    expect(screen.getByText('No models found. Create one to get started.')).toBeInTheDocument()
  })

  it('filters and renders only models matching the provider', () => {
    vi.mocked(useCLIModels).mockReturnValue({
      data: [
        makeModel({ id: 'claude-model', cli_type: 'claude' }),
        makeModel({ id: 'gemini-model', cli_type: 'gemini' }),
      ],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useCLIModels>)

    renderList('claude')
    expect(screen.getByText('claude-model')).toBeInTheDocument()
    expect(screen.queryByText('gemini-model')).not.toBeInTheDocument()
  })

  it('gemini badge uses amber color classes', () => {
    vi.mocked(useCLIModels).mockReturnValue({
      data: [makeModel({ id: 'gemini-flash', cli_type: 'gemini' })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useCLIModels>)

    renderList('gemini')
    const badge = screen.getByText('gemini')
    expect(badge.className).toMatch(/amber/)
  })

  it('claude badge uses blue color classes', () => {
    vi.mocked(useCLIModels).mockReturnValue({
      data: [makeModel()],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useCLIModels>)

    renderList('claude')
    const badge = screen.getByText('claude')
    expect(badge.className).toMatch(/blue/)
  })

  it('shows loading state', () => {
    vi.mocked(useCLIModels).mockReturnValue({
      data: [],
      isLoading: true,
      error: null,
    } as ReturnType<typeof useCLIModels>)

    renderList('claude')
    expect(screen.getByText('Loading models...')).toBeInTheDocument()
  })

  it('shows error message', () => {
    vi.mocked(useCLIModels).mockReturnValue({
      data: [],
      isLoading: false,
      error: new Error('fetch failed'),
    } as ReturnType<typeof useCLIModels>)

    renderList('claude')
    expect(screen.getByText('Error: fetch failed')).toBeInTheDocument()
  })

  it('New Model button shows create form', async () => {
    renderList('claude')
    await userEvent.click(screen.getByRole('button', { name: /new model/i }))
    expect(screen.getByTestId('cli-model-form')).toBeInTheDocument()
    expect(screen.getByText('create-form')).toBeInTheDocument()
  })

  it('cancel in create form hides it', async () => {
    renderList('claude')
    await userEvent.click(screen.getByRole('button', { name: /new model/i }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByTestId('cli-model-form')).not.toBeInTheDocument()
  })

  it('clicking the edit button opens edit form for the model', async () => {
    vi.mocked(useCLIModels).mockReturnValue({
      data: [makeModel({ id: 'my-model' })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useCLIModels>)

    renderList('claude')
    const row = screen.getByText('my-model').closest('.border')!
    const rowBtns = within(row as HTMLElement).getAllByRole('button')
    // rowBtns: [Check, Edit(pencil), Delete(trash)]
    await userEvent.click(rowBtns[1])
    expect(screen.getByTestId('cli-model-form')).toBeInTheDocument()
  })

  it('delete button absent for read_only models', () => {
    vi.mocked(useCLIModels).mockReturnValue({
      data: [makeModel({ id: 'builtin', read_only: true })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useCLIModels>)

    renderList('claude')
    const row = screen.getByText('builtin').closest('.border')!
    const rowBtns = within(row as HTMLElement).getAllByRole('button')
    // read_only row: [Check, Edit] — no trash button
    expect(rowBtns).toHaveLength(2)
  })

  it('clicking delete shows confirmation then calls deleteCLIModel', async () => {
    vi.mocked(useCLIModels).mockReturnValue({
      data: [makeModel({ id: 'my-model' })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useCLIModels>)
    vi.mocked(cliModelsApi.deleteCLIModel).mockResolvedValue({ status: 'ok' })

    renderList('claude')
    const row = screen.getByText('my-model').closest('.border')!
    const rowBtns = within(row as HTMLElement).getAllByRole('button')
    await userEvent.click(rowBtns[2]) // trash button

    expect(screen.getByText(/are you sure/i)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => {
      expect(cliModelsApi.deleteCLIModel).toHaveBeenCalledWith('my-model')
    })
  })

  it('toggle calls updateCLIModel with inverted enabled state', async () => {
    vi.mocked(useCLIModels).mockReturnValue({
      data: [makeModel({ id: 'my-model', enabled: true })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useCLIModels>)
    vi.mocked(cliModelsApi.updateCLIModel).mockResolvedValue({ status: 'ok' })

    renderList('claude')
    await userEvent.click(screen.getByRole('switch'))

    await waitFor(() => {
      expect(cliModelsApi.updateCLIModel).toHaveBeenCalledWith('my-model', { enabled: false })
    })
  })
})
