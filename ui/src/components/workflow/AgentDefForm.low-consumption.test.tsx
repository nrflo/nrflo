import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

vi.mock('@/hooks/useCLIModels', () => ({
  useModelOptions: () => [
    { label: 'Claude', options: [
      { value: 'haiku', label: 'Claude: Haiku' },
      { value: 'opus', label: 'Claude: Opus' },
      { value: 'opus_1m', label: 'Claude: Opus 1M' },
      { value: 'sonnet', label: 'Claude: Sonnet' },
    ]},
    { label: 'Codex', options: [
      { value: 'codex_gpt_high', label: 'Codex: GPT (High)' },
      { value: 'codex_gpt_normal', label: 'Codex: GPT (Normal)' },
      { value: 'codex_gpt54_high', label: 'Codex: GPT-54 (High)' },
      { value: 'codex_gpt54_normal', label: 'Codex: GPT-54 (Normal)' },
    ]},
    { label: 'OpenCode', options: [
      { value: 'opencode_gpt_high', label: 'OpenCode: GPT (High)' },
      { value: 'opencode_gpt_normal', label: 'OpenCode: GPT (Normal)' },
    ]},
  ],
}))

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value, onChange, placeholder }: any) => (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      aria-label="Prompt Template"
    />
  ),
}))

function renderForm(props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}) {
  const defaultProps = { isCreate: true, onSubmit: vi.fn(), onCancel: vi.fn(), ...props }
  return { ...render(<AgentDefForm {...defaultProps} />), props: defaultProps }
}

function getLCDropdownButton() {
  const label = screen.getByText('Low consumption model')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

async function selectDropdownOption(
  user: ReturnType<typeof userEvent.setup>,
  triggerButton: HTMLButtonElement,
  optionLabel: string
) {
  await user.click(triggerButton)
  const container = triggerButton.closest('.relative')!
  const option = Array.from(container.querySelectorAll('.cursor-pointer span')).find(
    (el) => el.textContent === optionLabel
  ) as HTMLElement
  await user.click(option)
}

describe('AgentDefForm - low consumption dropdown', () => {
  describe('visibility', () => {
    it('shows dropdown with model options', () => {
      renderForm()
      expect(screen.getByText('Low consumption model')).toBeInTheDocument()
    })

    it('shows helper text', () => {
      renderForm()
      expect(screen.getByText(/model to use when low consumption mode is enabled/i)).toBeInTheDocument()
    })
  })

  describe('options', () => {
    it('shows (none) plus model options', async () => {
      const user = userEvent.setup()
      renderForm()
      const btn = getLCDropdownButton()
      await user.click(btn)
      const container = btn.closest('.relative')!
      const options = Array.from(container.querySelectorAll('.cursor-pointer span')).map((el) => el.textContent)
      expect(options).toContain('(none)')
      expect(options).toContain('Claude: Sonnet')
      expect(options).toContain('Claude: Haiku')
      expect(options).toContain('Claude: Opus')
    })

    it('defaults to (none) when no initial value', () => {
      renderForm()
      expect(getLCDropdownButton().textContent).toContain('(none)')
    })
  })

  describe('selection and submission', () => {
    it('submits selected model as low_consumption_model', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Test prompt')
      await selectDropdownOption(user, getLCDropdownButton(), 'Claude: Sonnet')
      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ low_consumption_model: 'sonnet' })
      )
    })

    it('submits undefined when (none) selected', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Test prompt')
      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ low_consumption_model: undefined })
      )
    })

    it('pre-selects initial low_consumption_model', () => {
      renderForm({
        isCreate: false,
        initial: { low_consumption_model: 'haiku' },
      })
      expect(getLCDropdownButton().textContent).toContain('Claude: Haiku')
    })

    it('allows clearing back to (none) in update mode', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { low_consumption_model: 'haiku', prompt: 'Test' },
        onSubmit,
      })

      expect(getLCDropdownButton().textContent).toContain('Claude: Haiku')
      await selectDropdownOption(user, getLCDropdownButton(), '(none)')
      await user.click(screen.getByRole('button', { name: /^save$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ low_consumption_model: undefined })
      )
    })
  })
})
