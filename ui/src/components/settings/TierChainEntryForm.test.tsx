import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useState } from 'react'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { TierChainEntryForm } from './TierChainEntryForm'
import * as modelsApi from '@/api/models'
import type { Model } from '@/api/models'
import type { SetTierChainEntry } from '@/api/tierModels'

vi.mock('@/api/models')

function makeModel(overrides: Partial<Model> = {}): Model {
  return {
    id: 'm-cli-only',
    provider: 'anthropic',
    display_name: 'CLI Only',
    cli_model: 'cli-only',
    api_model: '',
    cli_efforts: [],
    api_efforts: [],
    cli_context: 200000,
    api_context: 200000,
    fallback_models: '',
    default_effort: '',
    read_only: false,
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const cliOnly = makeModel({ id: 'm-cli-only', display_name: 'CLI Only', cli_model: 'cli-only', api_model: '' })
const apiOnly = makeModel({ id: 'm-api-only', display_name: 'API Only', cli_model: '', api_model: 'api-only' })
const both = makeModel({ id: 'm-both', display_name: 'Both Modes', cli_model: 'both-cli', api_model: 'both-api' })

function renderEntry(entry: SetTierChainEntry, onChange = vi.fn()) {
  return { ...renderWithQuery(<TierChainEntryForm entry={entry} onChange={onChange} />), onChange }
}

/** Stateful wrapper so onChange updates are reflected back into the rendered entry, like TierChainRow does. */
function ControlledEntryForm({ initial, onChangeSpy }: { initial: SetTierChainEntry; onChangeSpy?: (e: SetTierChainEntry) => void }) {
  const [entry, setEntry] = useState(initial)
  return (
    <TierChainEntryForm
      entry={entry}
      onChange={(next) => {
        onChangeSpy?.(next)
        setEntry(next)
      }}
    />
  )
}

function renderControlledEntry(initial: SetTierChainEntry, onChangeSpy?: (e: SetTierChainEntry) => void) {
  return renderWithQuery(<ControlledEntryForm initial={initial} onChangeSpy={onChangeSpy} />)
}

function getModelDropdownButton() {
  return screen.getByText('Model').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

describe('TierChainEntryForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(modelsApi.listModels).mockResolvedValue([cliOnly, apiOnly, both])
  })

  it('cli_interactive mode shows only cli-enabled models', async () => {
    renderEntry({ execution_mode: 'cli_interactive', model_id: '', reasoning_effort: '' })
    const user = userEvent.setup()
    await screen.findByText('Select a model')
    await user.click(getModelDropdownButton())

    expect(screen.getByText(/CLI Only/)).toBeInTheDocument()
    expect(screen.getByText(/Both Modes/)).toBeInTheDocument()
    expect(screen.queryByText(/API Only/)).not.toBeInTheDocument()
  })

  it('selecting "Inherit (agent mode)" restricts the model list to the cli ∩ api intersection', async () => {
    renderControlledEntry({ execution_mode: 'cli_interactive', model_id: '', reasoning_effort: '' })
    const user = userEvent.setup()
    await screen.findByText('Select a model')

    const modeButton = screen.getByText('Mode').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    await user.click(modeButton)
    await user.click(screen.getByText('Inherit (agent mode)'))

    await user.click(getModelDropdownButton())
    const optionsContainer = getModelDropdownButton().parentElement!.querySelector('.absolute')!
    expect(within(optionsContainer).getByText(/Both Modes/)).toBeInTheDocument()
    expect(within(optionsContainer).queryByText(/CLI Only/)).not.toBeInTheDocument()
    expect(within(optionsContainer).queryByText(/API Only/)).not.toBeInTheDocument()
  })

  it('switching mode clears the previously selected model_id', async () => {
    const onChange = vi.fn()
    renderEntry({ execution_mode: 'cli_interactive', model_id: 'm-cli-only', reasoning_effort: '' }, onChange)
    const user = userEvent.setup()
    await screen.findByText('Select a model')

    const modeButton = screen.getByText('Mode').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    await user.click(modeButton)
    await user.click(screen.getByText('Inherit (agent mode)'))

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ execution_mode: '', model_id: '' }))
  })

  it('shows the derived read-only provider badge for the selected model', async () => {
    renderEntry({ execution_mode: 'cli_interactive', model_id: 'm-cli-only', reasoning_effort: '' })
    await screen.findByText('Anthropic')
    expect(screen.getByText('Anthropic')).toBeInTheDocument()
  })
})
