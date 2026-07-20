import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Model } from '@/api/models'
import * as hooks from '@/hooks/useModels'
import { ModelsList } from './ModelsList'

vi.mock('@/hooks/useModels')
vi.mock('./CLIModelCheckButton', () => ({ CLIModelCheckButton: () => <span>Check model</span> }))

const anthropic: Model = {
  id: 'sonnet-5', provider: 'anthropic', display_name: 'Sonnet', cli_model: 'sonnet', api_model: 'sonnet',
  cli_efforts: ['high'], api_efforts: ['high'], cli_context: 1000000, api_context: 1000000,
  fallback_models: '', default_effort: '', read_only: true, enabled: true, created_at: '', updated_at: '',
}
const openai: Model = { ...anthropic, id: 'gpt-5.4', provider: 'openai', display_name: 'GPT', api_model: '' }
const openrouter: Model = {
  ...anthropic, id: 'kimi-k3', provider: 'openrouter', display_name: 'Kimi', cli_model: '', api_model: 'moonshotai/kimi-k3',
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
})
