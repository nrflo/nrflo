import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { AgentDefForm } from './AgentDefForm'
import { useTierModels } from '@/hooks/useTierModels'
import type { TierModel } from '@/api/tierModels'

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => true,
}))

vi.mock('@/hooks/useDefaultTemplates', () => ({
  useInjectableTemplates: () => ({ data: [] }),
}))

vi.mock('@/hooks/useModels', () => ({
  useModelOptions: () => [
    {
      label: 'Anthropic',
      options: [
        { value: 'sonnet-5', label: 'Anthropic: Sonnet' },
        { value: 'opus-4-8', label: 'Anthropic: Opus' },
      ],
    },
  ],
  useModels: () => ({ data: [] }),
}))

vi.mock('@/hooks/useTierModels', async () => {
  const actual = await vi.importActual<typeof import('@/hooks/useTierModels')>('@/hooks/useTierModels')
  return { ...actual, useTierModels: vi.fn() }
})

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value, onChange, placeholder }: any) => (
    <textarea value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} aria-label="Prompt Template" />
  ),
}))

function makeRow(overrides: Partial<TierModel> = {}): TierModel {
  return {
    tier: 2,
    position: 0,
    provider: 'anthropic',
    execution_mode: 'cli_interactive',
    model_id: 'sonnet-5',
    reasoning_effort: '',
    ...overrides,
  }
}

function renderForm(props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}) {
  const defaultProps = { isCreate: true, onSubmit: vi.fn(), onCancel: vi.fn(), ...props }
  return renderWithQuery(<AgentDefForm {...defaultProps} />)
}

function getTierDropdownButton() {
  return screen.getByText('Tier').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

describe('AgentDefForm - tier UX', () => {
  it('toggle OFF submits model: "" and tier: N, showing the resolved chain primary', async () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [makeRow({ tier: 2, model_id: 'opus-4-8' })] } as any)
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    renderForm({ onSubmit })

    await user.click(getTierDropdownButton())
    await user.click(screen.getByText('Tier 2'))

    expect(screen.getByText(/Resolved model:/).parentElement).toHaveTextContent('opus-4-8')

    await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
    await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ model: '', tier: 2 }))
  })

  it('toggle ON submits the chosen model with tier: null', async () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [] } as any)
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    renderForm({ onSubmit })

    await user.click(screen.getByRole('switch', { name: /override model/i }))
    await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
    await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ model: 'sonnet-5', tier: null }))
  })

  it('switching execution mode while overriding re-picks a valid model', async () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [] } as any)
    const user = userEvent.setup()
    renderForm({ initial: { model: 'sonnet-5', prompt: 'test' } })

    const modelButton = screen.getByText('Model').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    expect(modelButton.textContent).toContain('Anthropic: Sonnet')

    // Switching to API mode re-derives model options; sonnet-5 stays valid here
    // since useModelOptions is mocked identically for both modes.
    const executionModeButton = screen.getByText('Execution Mode').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    await user.click(executionModeButton)
    await user.click(screen.getByText('API (in-process Anthropic runner)'))

    expect(screen.getByText('Model').parentElement!.querySelector('button[type="button"]')).toBeInTheDocument()
  })

  it('editing a tier-only def (model === "") opens with the toggle off and tier preselected', () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [] } as any)
    renderForm({ isCreate: false, initial: { model: '', tier: 3, prompt: 'test' } })

    expect(screen.getByRole('switch', { name: /override model/i })).toHaveAttribute('aria-checked', 'false')
    expect(getTierDropdownButton().textContent).toContain('Tier 3')
    expect(screen.queryByText('Model')).not.toBeInTheDocument()
  })

  it('editing an override def (non-empty model) opens with the toggle on', () => {
    vi.mocked(useTierModels).mockReturnValue({ data: [] } as any)
    renderForm({ isCreate: false, initial: { model: 'opus-4-8', tier: null, prompt: 'test' } })

    expect(screen.getByRole('switch', { name: /override model/i })).toHaveAttribute('aria-checked', 'true')
    const modelButton = screen.getByText('Model').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    expect(modelButton.textContent).toContain('Anthropic: Opus')
  })
})
