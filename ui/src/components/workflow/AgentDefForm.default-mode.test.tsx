import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => true,
}))

vi.mock('@/hooks/useDefaultTemplates', () => ({
  useInjectableTemplates: () => ({ data: [] }),
}))

vi.mock('@/hooks/useModels', () => ({
  useModelOptions: () => [
    { label: 'Anthropic', options: [
      { value: 'sonnet-5', label: 'Anthropic: Sonnet' },
    ]},
    { label: 'OpenAI', options: [
      { value: 'gpt-5.3-codex', label: 'OpenAI: GPT 5.3 Codex' },
    ]},
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
  return renderWithQuery(
    <AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} {...props} />
  )
}

function getExecutionModeButton() {
  return screen.getByText('Execution Mode')
    .parentElement!
    .querySelector('button[type="button"]') as HTMLButtonElement
}

describe('AgentDefForm — execution_mode default', () => {
  it('submitting without touching execution mode sends execution_mode: cli_interactive', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ isCreate: true, onSubmit })

    await user.type(screen.getByPlaceholderText(/e\.g\., setup-analyzer/i), 'my-agent')
    await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Some prompt')
    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ execution_mode: 'cli_interactive' })
    )
  })

  it('execution mode button shows CLI Interactive (PTY) by default', () => {
    renderForm()
    expect(getExecutionModeButton().textContent).toContain('CLI Interactive (PTY)')
  })

  describe('with initial codex model — no auto-coercion to cli', () => {
    it('execution mode button still shows CLI Interactive (PTY)', () => {
      renderForm({ isCreate: false, initial: { model: 'gpt-5.3-codex' } })
      expect(getExecutionModeButton().textContent).toContain('CLI Interactive (PTY)')
    })

    it('submitting with codex model sends execution_mode: cli_interactive', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { model: 'gpt-5.3-codex', prompt: 'Existing prompt' },
        onSubmit,
      })

      await user.click(screen.getByRole('button', { name: /save/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          model: 'gpt-5.3-codex',
          execution_mode: 'cli_interactive',
        })
      )
    })
  })
})
