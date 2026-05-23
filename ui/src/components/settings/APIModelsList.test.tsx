import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/utils'
import { APIModelsList } from './APIModelsList'
import type { APIModel } from '@/api/apiModels'

vi.mock('@/hooks/useAPIModels', () => ({
  useAPIModels: vi.fn(),
  apiModelKeys: { list: () => ['api-models', 'list'] },
}))

vi.mock('@/api/apiModels', () => ({
  createAPIModel: vi.fn(),
  updateAPIModel: vi.fn(),
  deleteAPIModel: vi.fn(),
}))

vi.mock('./APIModelForm', () => ({
  APIModelForm: ({
    onCancel,
    onSave,
    isCreate,
  }: {
    onCancel: () => void
    onSave: () => void
    isCreate?: boolean
    [k: string]: unknown
  }) => (
    <div data-testid="api-model-form">
      {isCreate && <span>create-form</span>}
      <button onClick={onCancel}>Cancel</button>
      <button onClick={onSave}>Save</button>
    </div>
  ),
  emptyAPIModelForm: {
    id: '',
    provider: 'anthropic',
    display_name: '',
    mapped_model: '',
    reasoning_effort: '',
    context_length: '200000',
  },
}))

import { useAPIModels } from '@/hooks/useAPIModels'
import * as apiModelsApi from '@/api/apiModels'

function makeModel(overrides: Partial<APIModel> = {}): APIModel {
  return {
    id: 'claude-opus',
    provider: 'anthropic',
    display_name: 'Claude Opus',
    mapped_model: 'claude-opus-4-7-20250514',
    reasoning_effort: '',
    context_length: 200000,
    read_only: false,
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderList(provider: 'anthropic' | 'openai' = 'anthropic') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <APIModelsList provider={provider} />
    </QueryClientProvider>
  )
}

describe('APIModelsList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAPIModels).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAPIModels>)
  })

  it('shows empty state when no models for provider', () => {
    renderList('anthropic')
    expect(screen.getByText('No models found. Create one to get started.')).toBeInTheDocument()
  })

  it('filters and renders only models matching the provider', () => {
    vi.mocked(useAPIModels).mockReturnValue({
      data: [
        makeModel({ id: 'anthropic-model', provider: 'anthropic' }),
        makeModel({ id: 'openai-model', provider: 'openai' }),
      ],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAPIModels>)

    renderList('anthropic')
    expect(screen.getByText('anthropic-model')).toBeInTheDocument()
    expect(screen.queryByText('openai-model')).not.toBeInTheDocument()
  })

  it('shows loading state', () => {
    vi.mocked(useAPIModels).mockReturnValue({
      data: [],
      isLoading: true,
      error: null,
    } as ReturnType<typeof useAPIModels>)

    renderList('anthropic')
    expect(screen.getByText('Loading models...')).toBeInTheDocument()
  })

  it('shows error message', () => {
    vi.mocked(useAPIModels).mockReturnValue({
      data: [],
      isLoading: false,
      error: new Error('fetch failed'),
    } as ReturnType<typeof useAPIModels>)

    renderList('anthropic')
    expect(screen.getByText('Error: fetch failed')).toBeInTheDocument()
  })

  it('New Model button shows create form', async () => {
    renderList('anthropic')
    await userEvent.click(screen.getByRole('button', { name: /new model/i }))
    expect(screen.getByTestId('api-model-form')).toBeInTheDocument()
    expect(screen.getByText('create-form')).toBeInTheDocument()
  })

  it('cancel in create form hides it', async () => {
    renderList('anthropic')
    await userEvent.click(screen.getByRole('button', { name: /new model/i }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByTestId('api-model-form')).not.toBeInTheDocument()
  })

  it('clicking edit button opens edit form', async () => {
    vi.mocked(useAPIModels).mockReturnValue({
      data: [makeModel({ id: 'my-model', provider: 'anthropic' })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAPIModels>)

    renderList('anthropic')
    const row = screen.getByText('my-model').closest('.border')!
    // api-models row has no Check button: [Edit, Delete]
    const rowBtns = within(row as HTMLElement).getAllByRole('button')
    await userEvent.click(rowBtns[0]) // Edit (pencil) — index 0
    expect(screen.getByTestId('api-model-form')).toBeInTheDocument()
  })

  it('delete button absent for read_only models', () => {
    vi.mocked(useAPIModels).mockReturnValue({
      data: [makeModel({ id: 'builtin', provider: 'anthropic', read_only: true })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAPIModels>)

    renderList('anthropic')
    const row = screen.getByText('builtin').closest('.border')!
    // read_only row: [Edit] — no trash button
    const rowBtns = within(row as HTMLElement).getAllByRole('button')
    expect(rowBtns).toHaveLength(1)
  })

  it('clicking delete shows confirmation then calls deleteAPIModel', async () => {
    vi.mocked(useAPIModels).mockReturnValue({
      data: [makeModel({ id: 'my-model', provider: 'anthropic' })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAPIModels>)
    vi.mocked(apiModelsApi.deleteAPIModel).mockResolvedValue({ status: 'ok' })

    renderList('anthropic')
    const row = screen.getByText('my-model').closest('.border')!
    const rowBtns = within(row as HTMLElement).getAllByRole('button')
    await userEvent.click(rowBtns[1]) // trash button — index 1

    expect(screen.getByText(/are you sure/i)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => {
      expect(apiModelsApi.deleteAPIModel).toHaveBeenCalledWith('my-model')
    })
  })

  it('toggle calls updateAPIModel with inverted enabled state', async () => {
    vi.mocked(useAPIModels).mockReturnValue({
      data: [makeModel({ id: 'my-model', provider: 'anthropic', enabled: true })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAPIModels>)
    vi.mocked(apiModelsApi.updateAPIModel).mockResolvedValue({ status: 'ok' })

    renderList('anthropic')
    await userEvent.click(screen.getByRole('switch'))

    await waitFor(() => {
      expect(apiModelsApi.updateAPIModel).toHaveBeenCalledWith('my-model', { enabled: false })
    })
  })

  it('toggle is disabled for read_only models', () => {
    vi.mocked(useAPIModels).mockReturnValue({
      data: [makeModel({ id: 'builtin', provider: 'anthropic', read_only: true, enabled: true })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAPIModels>)

    renderList('anthropic')
    expect(screen.getByRole('switch')).toBeDisabled()
  })

  it('saving a read_only model sends only reasoning_effort', async () => {
    vi.mocked(useAPIModels).mockReturnValue({
      data: [makeModel({ id: 'builtin', provider: 'anthropic', read_only: true })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAPIModels>)
    vi.mocked(apiModelsApi.updateAPIModel).mockResolvedValue({ status: 'ok' })

    renderList('anthropic')
    const row = screen.getByText('builtin').closest('.border')!
    const rowBtns = within(row as HTMLElement).getAllByRole('button')
    await userEvent.click(rowBtns[0]) // Edit
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(apiModelsApi.updateAPIModel).toHaveBeenCalledWith('builtin', {
        reasoning_effort: '',
      })
    })
  })

  it('anthropic badge uses blue color classes', () => {
    vi.mocked(useAPIModels).mockReturnValue({
      data: [makeModel({ provider: 'anthropic' })],
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAPIModels>)

    renderList('anthropic')
    const badge = screen.getByText('anthropic')
    expect(badge.className).toMatch(/blue/)
  })
})
