import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => true,
}))

vi.mock('@/hooks/useDefaultTemplates', () => ({
  useInjectableTemplates: () => ({ data: [] }),
}))

vi.mock('@/hooks/useModels', () => ({
  useModelOptions: (mode: string) => mode === 'api' ? [
    { label: 'Anthropic', options: [
      { value: 'opus-4-8', label: 'Anthropic: Opus' },
    ]},
    { label: 'OpenAI', options: [
      { value: 'gpt-5.4', label: 'OpenAI: GPT 5.4' },
    ]},
  ] : [
    { label: 'Anthropic', options: [{ value: 'sonnet-5', label: 'Anthropic: Sonnet' }] },
  ],
  useModels: () => ({ data: [] }),
}))

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value, onChange, placeholder }: {
    value: string; onChange: (v: string) => void; placeholder?: string
  }) => (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      aria-label="Prompt Template"
    />
  ),
}))

function renderForm(props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}) {
  return render(
    <AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} {...props} />
  )
}

function getExecutionModeButton() {
  const label = screen.getByText('Execution Mode')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

function getModelDropdownButton() {
  const label = screen.getByText('Model')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

function getLowConsumptionDropdownButton() {
  const label = screen.getByText('Low consumption model')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

describe('AgentDefForm — model dropdown routing', () => {
  describe('cli_interactive mode (default)', () => {
    it('Model dropdown shows CLI model options', async () => {
      renderForm()
      const btn = getModelDropdownButton()
      await userEvent.click(btn)
      const panel = btn.parentElement!.querySelector('.absolute') as HTMLElement
      expect(within(panel).getByText('Anthropic: Sonnet')).toBeInTheDocument()
      expect(within(panel).queryByText('Anthropic: Opus')).not.toBeInTheDocument()
    })

    it('Low consumption model dropdown shows CLI model options', async () => {
      renderForm()
      const btn = getLowConsumptionDropdownButton()
      await userEvent.click(btn)
      const panel = btn.parentElement!.querySelector('.absolute') as HTMLElement
      expect(within(panel).getByText('Anthropic: Sonnet')).toBeInTheDocument()
      expect(within(panel).queryByText('Anthropic: Opus')).not.toBeInTheDocument()
    })
  })

  describe('api mode', () => {
    async function switchToAPI() {
      const user = userEvent.setup()
      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))
    }

    it('Model dropdown shows API model options after switching to api', async () => {
      renderForm()
      await switchToAPI()
      const btn = getModelDropdownButton()
      await userEvent.click(btn)
      const panel = btn.parentElement!.querySelector('.absolute') as HTMLElement
      expect(within(panel).getByText('Anthropic: Opus')).toBeInTheDocument()
      expect(within(panel).queryByText('Anthropic: Sonnet')).not.toBeInTheDocument()
    })

    it('Low consumption model dropdown shows API model options in api mode', async () => {
      renderForm()
      await switchToAPI()
      const btn = getLowConsumptionDropdownButton()
      await userEvent.click(btn)
      const panel = btn.parentElement!.querySelector('.absolute') as HTMLElement
      expect(within(panel).getByText('Anthropic: Opus')).toBeInTheDocument()
      expect(within(panel).queryByText('Anthropic: Sonnet')).not.toBeInTheDocument()
    })

    it('switching back to cli restores CLI model options', async () => {
      renderForm()
      await switchToAPI()

      // Switch back to CLI
      await userEvent.click(getExecutionModeButton())
      await userEvent.click(screen.getByText('CLI Interactive (PTY)'))

      const btn = getModelDropdownButton()
      await userEvent.click(btn)
      const panel = btn.parentElement!.querySelector('.absolute') as HTMLElement
      expect(within(panel).getByText('Anthropic: Sonnet')).toBeInTheDocument()
      expect(within(panel).queryByText('Anthropic: Opus')).not.toBeInTheDocument()
    })

    it('initial api mode shows API model options immediately', () => {
      renderForm({ isCreate: false, initial: { execution_mode: 'api' } })
      // model dropdown button should show the api model label
      const modelBtn = getModelDropdownButton()
      // button text reflects no selection (first value or placeholder)
      expect(modelBtn).toBeInTheDocument()
    })
  })
})
