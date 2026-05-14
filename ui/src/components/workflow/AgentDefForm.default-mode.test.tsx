import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => true,
}))

vi.mock('@/hooks/useCLIModels', () => ({
  useModelOptions: () => [
    { label: 'Claude', options: [
      { value: 'sonnet', label: 'Claude: Sonnet' },
    ]},
    { label: 'OpenCode', options: [
      { value: 'opencode_gpt54', label: 'OpenCode: GPT 5.4' },
    ]},
  ],
  useCLIModels: () => ({ data: [] }),
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

  describe('with initial opencode model — no auto-coercion to cli', () => {
    it('execution mode button still shows CLI Interactive (PTY)', () => {
      renderForm({ isCreate: false, initial: { model: 'opencode_gpt54' } })
      expect(getExecutionModeButton().textContent).toContain('CLI Interactive (PTY)')
    })

    it('submitting with opencode model sends execution_mode: cli_interactive', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { model: 'opencode_gpt54', prompt: 'Existing prompt' },
        onSubmit,
      })

      await user.click(screen.getByRole('button', { name: /save/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          model: 'opencode_gpt54',
          execution_mode: 'cli_interactive',
        })
      )
    })
  })
})
