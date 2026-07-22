import { describe, expect, it, vi, beforeEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ModelsProviderTabs, BUILTIN_PROVIDERS, BUILTIN_PROVIDER_IDS } from './ModelsProviderTabs'
import * as customProviderHooks from '@/hooks/useCustomProviders'
import type { CustomProvider } from '@/api/customProviders'

vi.mock('@/hooks/useCustomProviders')
vi.mock('./ProviderConnectionCheckButton', () => ({ ProviderConnectionCheckButton: () => <span>Check connection</span> }))

const acme: CustomProvider = {
  name: 'acme', base_url: 'https://acme.test', api_key: 'k', api_wire: 'responses', enabled: true, created_at: '', updated_at: '',
}

describe('BUILTIN_PROVIDERS / BUILTIN_PROVIDER_IDS', () => {
  it('is anthropic/openai/openrouter', () => {
    expect(BUILTIN_PROVIDERS.map((p) => p.id)).toEqual(['anthropic', 'openai', 'openrouter'])
    expect([...BUILTIN_PROVIDER_IDS]).toEqual(['anthropic', 'openai', 'openrouter'])
  })
})

describe('ModelsProviderTabs', () => {
  const createMutation = { mutate: vi.fn(), isPending: false, isError: false, error: null }

  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(customProviderHooks.useCustomProviders).mockReturnValue({ data: [acme], isLoading: false, error: null } as never)
    vi.mocked(customProviderHooks.useCreateCustomProvider).mockReturnValue(createMutation as never)
  })

  it('renders built-in tabs plus a registered custom provider', () => {
    render(<ModelsProviderTabs activeProvider="anthropic" onSelect={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'Anthropic' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'OpenAI' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'OpenRouter' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'acme' })).toBeInTheDocument()
  })

  it('clicking a tab calls onSelect with its provider id', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(<ModelsProviderTabs activeProvider="anthropic" onSelect={onSelect} />)
    await user.click(screen.getByRole('button', { name: 'acme' }))
    expect(onSelect).toHaveBeenCalledWith('acme')
  })

  it('Add provider opens the create form; Cancel closes it', async () => {
    const user = userEvent.setup()
    render(<ModelsProviderTabs activeProvider="anthropic" onSelect={vi.fn()} />)
    expect(screen.queryByRole('button', { name: 'Create' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /add provider/i }))
    expect(screen.getByRole('button', { name: 'Create' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('button', { name: 'Create' })).not.toBeInTheDocument()
  })

  it('saving the create form calls useCreateCustomProvider.mutate and selects the new provider on success', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(<ModelsProviderTabs activeProvider="anthropic" onSelect={onSelect} />)
    await user.click(screen.getByRole('button', { name: /add provider/i }))
    await user.type(screen.getByPlaceholderText('my-provider'), 'new-provider')
    await user.type(screen.getByPlaceholderText('https://api.example.com/v1'), 'https://new.test')
    await user.type(screen.getByPlaceholderText(''), 'sk-key')
    await user.click(screen.getByRole('button', { name: 'Create' }))
    expect(createMutation.mutate).toHaveBeenCalledWith(
      { name: 'new-provider', base_url: 'https://new.test', api_key: 'sk-key', api_wire: 'responses' },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    )
    const [, opts] = createMutation.mutate.mock.calls[0]
    act(() => opts.onSuccess({ ...acme, name: 'new-provider' }))
    expect(onSelect).toHaveBeenCalledWith('new-provider')
    expect(screen.queryByRole('button', { name: 'Create' })).not.toBeInTheDocument()
  })
})
