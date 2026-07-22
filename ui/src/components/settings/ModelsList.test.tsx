import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Model } from '@/api/models'
import type { CustomProvider } from '@/api/customProviders'
import * as hooks from '@/hooks/useModels'
import * as customProviderHooks from '@/hooks/useCustomProviders'
import { ModelsList } from './ModelsList'

vi.mock('@/hooks/useModels')
vi.mock('@/hooks/useCustomProviders')
vi.mock('./CLIModelCheckButton', () => ({ CLIModelCheckButton: () => <span>Check model</span> }))
vi.mock('./ProviderConnectionCheckButton', () => ({ ProviderConnectionCheckButton: () => <span>Check connection</span> }))

const anthropic: Model = {
  id: 'sonnet-5', provider: 'anthropic', display_name: 'Sonnet', cli_model: 'sonnet', api_model: 'sonnet',
  cli_efforts: ['high'], api_efforts: ['high'], cli_context: 1000000, api_context: 1000000,
  fallback_models: '', default_effort: '', read_only: true, enabled: true, created_at: '', updated_at: '',
}
const openai: Model = { ...anthropic, id: 'gpt-5.4', provider: 'openai', display_name: 'GPT', api_model: '' }
const openrouter: Model = {
  ...anthropic, id: 'kimi-k3', provider: 'openrouter', display_name: 'Kimi', cli_model: '', api_model: 'moonshotai/kimi-k3',
}
const acmeModel: Model = {
  ...anthropic, id: 'acme-1', provider: 'acme', display_name: 'Acme One', cli_model: '', api_model: 'acme-model-1', read_only: false,
}
const acmeProvider: CustomProvider = {
  name: 'acme', base_url: 'https://acme.test', api_key: 'stored-key', api_wire: 'responses', enabled: true, created_at: '', updated_at: '',
}

describe('ModelsList', () => {
  const create = { mutate: vi.fn(), isPending: false, isError: false, error: null }
  const update = { mutate: vi.fn(), isPending: false, isError: false, error: null }
  const remove = { mutate: vi.fn(), isPending: false, isError: false, error: null }

  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(hooks.useModels).mockReturnValue({ data: [anthropic, openai, openrouter], isLoading: false, error: null } as never)
    vi.mocked(hooks.useCreateModel).mockReturnValue(create as never)
    vi.mocked(hooks.useUpdateModel).mockReturnValue(update as never)
    vi.mocked(hooks.useDeleteModel).mockReturnValue(remove as never)
    vi.mocked(customProviderHooks.useCustomProviders).mockReturnValue({ data: [], isLoading: false, error: null } as never)
    vi.mocked(customProviderHooks.useUpdateCustomProvider).mockReturnValue(update as never)
    vi.mocked(customProviderHooks.useDeleteCustomProvider).mockReturnValue(remove as never)
  })

  it('filters by provider and shows supported-mode badges plus the CLI test action', () => {
    render(<ModelsList provider="anthropic" />)
    expect(screen.getByText('sonnet-5')).toBeInTheDocument()
    expect(screen.queryByText('gpt-5.4')).not.toBeInTheDocument()
    expect(screen.getByText('CLI ✓')).toBeInTheDocument()
    expect(screen.getByText('API ✓')).toBeInTheDocument()
    expect(screen.getByText('Check model')).toBeInTheDocument()
  })

  it('shows only the API badge and hides the CLI test action for an openrouter row', () => {
    render(<ModelsList provider="openrouter" />)
    expect(screen.getByText('kimi-k3')).toBeInTheDocument()
    expect(screen.queryByText('sonnet-5')).not.toBeInTheDocument()
    expect(screen.queryByText('CLI ✓')).not.toBeInTheDocument()
    expect(screen.getByText('API ✓')).toBeInTheDocument()
    expect(screen.queryByText('Check model')).not.toBeInTheDocument()
  })

  it('starts a new model form with the active provider', async () => {
    const user = userEvent.setup()
    render(<ModelsList provider="openai" />)
    await user.click(screen.getByRole('button', { name: 'New Model' }))
    expect(screen.getByDisplayValue('openai')).toBeInTheDocument()
  })

  describe('registered custom provider', () => {
    beforeEach(() => {
      vi.mocked(hooks.useModels).mockReturnValue({ data: [anthropic, acmeModel], isLoading: false, error: null } as never)
      vi.mocked(customProviderHooks.useCustomProviders).mockReturnValue({ data: [acmeProvider], isLoading: false, error: null } as never)
    })

    it('is apiOnly: hides the CLI badge/test action, shows only the API badge', () => {
      render(<ModelsList provider="acme" />)
      expect(screen.getByText('acme-1')).toBeInTheDocument()
      expect(screen.queryByText('CLI ✓')).not.toBeInTheDocument()
      expect(screen.getByText('API ✓')).toBeInTheDocument()
      expect(screen.queryByText('Check model')).not.toBeInTheDocument()
    })

    it('renders a CustomProviderCard above the model list for the active custom provider', () => {
      render(<ModelsList provider="acme" />)
      expect(screen.getByText('acme')).toBeInTheDocument()
      expect(screen.getByText(/https:\/\/acme\.test/)).toBeInTheDocument()
    })

    it('does not render a CustomProviderCard for a built-in provider', () => {
      vi.mocked(hooks.useModels).mockReturnValue({ data: [anthropic], isLoading: false, error: null } as never)
      render(<ModelsList provider="anthropic" />)
      expect(screen.queryByText('https://acme.test')).not.toBeInTheDocument()
    })

    it('editing the provider opens CustomProviderForm seeded with its values', async () => {
      const user = userEvent.setup()
      render(<ModelsList provider="acme" />)
      const card = screen.getByText('acme').closest('div')!.parentElement!.parentElement!
      await user.click(within(card).getAllByRole('button')[0])
      expect(screen.getByDisplayValue('https://acme.test')).toBeInTheDocument()
    })

    it('saving an edit with api_key left blank omits it from the update payload', async () => {
      const user = userEvent.setup()
      render(<ModelsList provider="acme" />)
      const card = screen.getByText('acme').closest('div')!.parentElement!.parentElement!
      await user.click(within(card).getAllByRole('button')[0])
      await user.click(screen.getByRole('button', { name: /Save/ }))
      expect(update.mutate).toHaveBeenCalledWith(
        { name: 'acme', data: { base_url: 'https://acme.test', api_wire: 'responses', enabled: true } },
        expect.objectContaining({ onSuccess: expect.any(Function) }),
      )
    })

    it('saving an edit with a new api_key includes it in the update payload', async () => {
      const user = userEvent.setup()
      render(<ModelsList provider="acme" />)
      const card = screen.getByText('acme').closest('div')!.parentElement!.parentElement!
      await user.click(within(card).getAllByRole('button')[0])
      await user.type(screen.getByPlaceholderText('Leave blank to keep current key'), 'new-key')
      await user.click(screen.getByRole('button', { name: /Save/ }))
      expect(update.mutate).toHaveBeenCalledWith(
        { name: 'acme', data: { base_url: 'https://acme.test', api_wire: 'responses', enabled: true, api_key: 'new-key' } },
        expect.objectContaining({ onSuccess: expect.any(Function) }),
      )
    })

    it('delete asks for confirmation and surfaces the BE 409 in-use message on failure', async () => {
      const user = userEvent.setup()
      const failingDelete = { mutate: vi.fn((_name, opts) => opts.onError(new Error('custom provider is in use by: acme-1'))), isPending: false, isError: false, error: null }
      vi.mocked(customProviderHooks.useDeleteCustomProvider).mockReturnValue(failingDelete as never)
      render(<ModelsList provider="acme" />)
      const card = screen.getByText('acme').closest('div')!.parentElement!.parentElement!
      const buttons = within(card).getAllByRole('button')
      await user.click(buttons[1])
      expect(screen.getByText(/Delete provider/)).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: 'Delete' }))
      expect(failingDelete.mutate).toHaveBeenCalledWith('acme', expect.objectContaining({ onError: expect.any(Function) }))
      expect(screen.getByText('custom provider is in use by: acme-1')).toBeInTheDocument()
    })
  })
})
