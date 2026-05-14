import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProviderModelsList } from './ProviderModelsList'
import { useCLIModels } from '@/hooks/useCLIModels'
import * as cliModelsApi from '@/api/cliModels'
import type { CLIModel } from '@/api/cliModels'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/hooks/useCLIModels', () => ({
  useCLIModels: vi.fn(),
  cliModelKeys: {
    all: ['cli-models'],
    list: () => ['cli-models', 'list'],
  },
}))
vi.mock('@/api/cliModels')
vi.mock('./CLIModelCheckButton', () => ({
  CLIModelCheckButton: () => null,
}))

function makeModel(overrides: Partial<CLIModel> = {}): CLIModel {
  return {
    id: 'test-model',
    cli_type: 'claude',
    display_name: 'Test Model',
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

function setModels(models: CLIModel[]) {
  vi.mocked(useCLIModels).mockReturnValue({ data: models, isLoading: false, error: null } as never)
}

describe('ProviderModelsList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(cliModelsApi.createCLIModel).mockResolvedValue(makeModel())
    vi.mocked(cliModelsApi.updateCLIModel).mockResolvedValue({ status: 'ok' })
    vi.mocked(cliModelsApi.deleteCLIModel).mockResolvedValue({ status: 'ok' })
  })

  it('shows loading state', () => {
    vi.mocked(useCLIModels).mockReturnValue({ data: undefined, isLoading: true, error: null } as never)
    renderWithQuery(<ProviderModelsList provider="claude" />)
    expect(screen.getByText(/loading models/i)).toBeInTheDocument()
  })

  it('shows empty state when no models for provider', () => {
    setModels([])
    renderWithQuery(<ProviderModelsList provider="claude" />)
    expect(screen.getByText(/no models found/i)).toBeInTheDocument()
  })

  describe('provider filtering', () => {
    const allModels = [
      makeModel({ id: 'claude-model', cli_type: 'claude' }),
      makeModel({ id: 'opencode-model', cli_type: 'opencode' }),
      makeModel({ id: 'codex-model', cli_type: 'codex' }),
      makeModel({ id: 'other-type-model', cli_type: 'other' }),
    ]

    it('shows only models matching provider=claude, hides all others', () => {
      setModels(allModels)
      renderWithQuery(<ProviderModelsList provider="claude" />)
      expect(screen.getByText('claude-model')).toBeInTheDocument()
      expect(screen.queryByText('opencode-model')).not.toBeInTheDocument()
      expect(screen.queryByText('codex-model')).not.toBeInTheDocument()
      expect(screen.queryByText('other-type-model')).not.toBeInTheDocument()
    })

    it('shows only models matching provider=opencode', () => {
      setModels(allModels)
      renderWithQuery(<ProviderModelsList provider="opencode" />)
      expect(screen.queryByText('claude-model')).not.toBeInTheDocument()
      expect(screen.getByText('opencode-model')).toBeInTheDocument()
      expect(screen.queryByText('other-type-model')).not.toBeInTheDocument()
    })

    it('shows only models matching provider=codex', () => {
      setModels(allModels)
      renderWithQuery(<ProviderModelsList provider="codex" />)
      expect(screen.queryByText('claude-model')).not.toBeInTheDocument()
      expect(screen.getByText('codex-model')).toBeInTheDocument()
      expect(screen.queryByText('other-type-model')).not.toBeInTheDocument()
    })

    it('never renders an Other section header or other-type models', () => {
      setModels(allModels)
      renderWithQuery(<ProviderModelsList provider="claude" />)
      expect(screen.queryByText('other-type-model')).not.toBeInTheDocument()
      expect(screen.queryByText('Other')).not.toBeInTheDocument()
    })
  })

  describe('create mutation', () => {
    it('shows create form when New Model is clicked', async () => {
      setModels([])
      renderWithQuery(<ProviderModelsList provider="claude" />)
      await userEvent.setup().click(screen.getByRole('button', { name: /new model/i }))
      expect(screen.getByPlaceholderText('my-custom-model')).toBeInTheDocument()
    })

    it('submits create form calling createCLIModel with entered values', async () => {
      setModels([])
      renderWithQuery(<ProviderModelsList provider="claude" />)
      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /new model/i }))
      await user.type(screen.getByPlaceholderText('my-custom-model'), 'new-model-id')
      await user.type(screen.getByPlaceholderText('My Model'), 'My New Model')
      await user.type(screen.getByPlaceholderText('claude-sonnet-4-20250514'), 'claude-sonnet-4-6')
      await user.click(screen.getByRole('button', { name: /create/i }))
      await waitFor(() => {
        expect(cliModelsApi.createCLIModel).toHaveBeenCalledWith(expect.objectContaining({
          id: 'new-model-id',
          display_name: 'My New Model',
          mapped_model: 'claude-sonnet-4-6',
          cli_type: 'claude',
        }))
      })
    })

    it('cancel button hides create form without calling createCLIModel', async () => {
      setModels([])
      renderWithQuery(<ProviderModelsList provider="claude" />)
      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /new model/i }))
      expect(screen.getByPlaceholderText('my-custom-model')).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: /cancel/i }))
      expect(screen.queryByPlaceholderText('my-custom-model')).not.toBeInTheDocument()
      expect(cliModelsApi.createCLIModel).not.toHaveBeenCalled()
    })
  })

  describe('delete mutation', () => {
    it('shows delete confirmation dialog with model id', async () => {
      const model = makeModel({ id: 'del-model', cli_type: 'claude', read_only: false })
      setModels([model])
      renderWithQuery(<ProviderModelsList provider="claude" />)
      // buttons: [New Model(0), Edit(1), Delete(2)] — Toggle is role="switch", not button
      const buttons = screen.getAllByRole('button')
      await userEvent.setup().click(buttons[2])
      expect(screen.getByText(/are you sure.*delete/i)).toBeInTheDocument()
      expect(screen.getByText('del-model')).toBeInTheDocument()
    })

    it('calls deleteCLIModel when Delete is confirmed', async () => {
      const model = makeModel({ id: 'del-model', cli_type: 'claude', read_only: false })
      setModels([model])
      renderWithQuery(<ProviderModelsList provider="claude" />)
      const user = userEvent.setup()
      const buttons = screen.getAllByRole('button')
      await user.click(buttons[2])
      await user.click(screen.getByRole('button', { name: /^delete$/i }))
      await waitFor(() => {
        expect(cliModelsApi.deleteCLIModel).toHaveBeenCalledWith('del-model')
      })
    })

    it('cancel in confirm dialog closes without deleting', async () => {
      const model = makeModel({ id: 'del-model', cli_type: 'claude', read_only: false })
      setModels([model])
      renderWithQuery(<ProviderModelsList provider="claude" />)
      const user = userEvent.setup()
      const buttons = screen.getAllByRole('button')
      await user.click(buttons[2])
      await user.click(screen.getByRole('button', { name: /cancel/i }))
      expect(screen.queryByText(/are you sure.*delete/i)).not.toBeInTheDocument()
      expect(cliModelsApi.deleteCLIModel).not.toHaveBeenCalled()
    })

    it('delete button is absent for read-only models', () => {
      const model = makeModel({ id: 'ro-model', cli_type: 'claude', read_only: true })
      setModels([model])
      renderWithQuery(<ProviderModelsList provider="claude" />)
      // read_only model has no delete button — only [New Model, Edit]
      const buttons = screen.getAllByRole('button')
      expect(buttons).toHaveLength(2)
    })
  })

  describe('toggle mutation', () => {
    it('calls updateCLIModel with enabled=false when toggling an enabled model', async () => {
      const model = makeModel({ id: 'tog-model', cli_type: 'claude', enabled: true, read_only: false })
      setModels([model])
      renderWithQuery(<ProviderModelsList provider="claude" />)
      const [toggle] = screen.getAllByRole('switch')
      await userEvent.setup().click(toggle)
      await waitFor(() => {
        expect(cliModelsApi.updateCLIModel).toHaveBeenCalledWith('tog-model', { enabled: false })
      })
    })

    it('calls updateCLIModel with enabled=true when toggling a disabled model', async () => {
      const model = makeModel({ id: 'tog-model', cli_type: 'claude', enabled: false, read_only: false })
      setModels([model])
      renderWithQuery(<ProviderModelsList provider="claude" />)
      const [toggle] = screen.getAllByRole('switch')
      await userEvent.setup().click(toggle)
      await waitFor(() => {
        expect(cliModelsApi.updateCLIModel).toHaveBeenCalledWith('tog-model', { enabled: true })
      })
    })

    it('toggle is disabled for read-only models', () => {
      const model = makeModel({ id: 'ro-model', cli_type: 'claude', read_only: true, enabled: true })
      setModels([model])
      renderWithQuery(<ProviderModelsList provider="claude" />)
      const [toggle] = screen.getAllByRole('switch')
      expect(toggle).toBeDisabled()
    })
  })
})
