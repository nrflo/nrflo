import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { AgentDefModelTierFields } from './AgentDefModelTierFields'
import { useTierModels } from '@/hooks/useTierModels'
import type { TierModel } from '@/api/tierModels'
import type { DropdownOptionGroup } from '@/components/ui/Dropdown'

vi.mock('@/hooks/useTierModels', async () => {
  const actual = await vi.importActual<typeof import('@/hooks/useTierModels')>('@/hooks/useTierModels')
  return { ...actual, useTierModels: vi.fn() }
})

vi.mock('@/hooks/useModels', () => ({ useModels: () => ({ data: [] }) }))

function makeRow(overrides: Partial<TierModel> = {}): TierModel {
  return {
    tier: 1,
    position: 0,
    provider: 'anthropic',
    execution_mode: 'cli_interactive',
    model_id: 'sonnet-5',
    reasoning_effort: '',
    ...overrides,
  }
}

const modelOptions: DropdownOptionGroup[] = [
  {
    label: 'Anthropic',
    options: [
      { value: 'sonnet-5', label: 'Anthropic: Sonnet' },
      { value: 'opus-4-8', label: 'Anthropic: Opus' },
    ],
  },
]

function renderFields(props: Partial<React.ComponentProps<typeof AgentDefModelTierFields>> = {}) {
  const defaultProps = {
    tier: 1,
    onTierChange: vi.fn(),
    override: false,
    onOverrideChange: vi.fn(),
    model: '',
    onModelChange: vi.fn(),
    executionMode: 'cli_interactive' as const,
    reasoningEffort: '',
    onReasoningEffortChange: vi.fn(),
    modelOptions,
    ...props,
  }
  return { ...renderWithQuery(<AgentDefModelTierFields {...defaultProps} />), props: defaultProps }
}

function getTierDropdownButton() {
  return screen.getByText('Tier').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

describe('AgentDefModelTierFields', () => {
  it('shows the resolved chain-primary model when override is off', () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [makeRow({ tier: 1, model_id: 'sonnet-5' })] } as any)
    renderFields({ override: false, tier: 1 })

    expect(screen.getByText(/Resolved model:/).parentElement).toHaveTextContent('sonnet-5')
    expect(screen.queryByText('Model')).not.toBeInTheDocument()
  })

  it('shows a graceful fallback when resolveTierChain returns []', () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [] } as any)
    renderFields({ override: false, tier: 3 })

    expect(screen.getByText(/no chain configured for this tier/)).toBeInTheDocument()
  })

  it('toggling override ON reveals the Model dropdown and picks a default model', async () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [] } as any)
    const onOverrideChange = vi.fn()
    const onModelChange = vi.fn()
    const user = userEvent.setup()
    renderFields({ override: false, model: '', onOverrideChange, onModelChange })

    await user.click(screen.getByRole('switch', { name: /override model/i }))

    expect(onOverrideChange).toHaveBeenCalledWith(true)
    expect(onModelChange).toHaveBeenCalledWith('sonnet-5')
  })

  it('toggling override OFF clears the model back to empty', async () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [] } as any)
    const onOverrideChange = vi.fn()
    const onModelChange = vi.fn()
    const user = userEvent.setup()
    renderFields({ override: true, model: 'opus-4-8', onOverrideChange, onModelChange })

    await user.click(screen.getByRole('switch', { name: /override model/i }))

    expect(onOverrideChange).toHaveBeenCalledWith(false)
    expect(onModelChange).toHaveBeenCalledWith('')
  })

  it('override ON renders the Model dropdown seeded with the current model', () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [] } as any)
    renderFields({ override: true, model: 'opus-4-8' })

    const label = screen.getByText('Model')
    const button = label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    expect(button.textContent).toContain('Anthropic: Opus')
  })

  it('changing the tier dropdown calls onTierChange with the numeric tier', async () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [] } as any)
    const onTierChange = vi.fn()
    const user = userEvent.setup()
    renderFields({ tier: 1, onTierChange })

    await user.click(getTierDropdownButton())
    await user.click(screen.getByText('Tier 2'))

    expect(onTierChange).toHaveBeenCalledWith(2)
  })
})
