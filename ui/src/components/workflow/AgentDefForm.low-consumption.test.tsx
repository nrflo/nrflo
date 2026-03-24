import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

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
  const label = screen.getByText('Low consumption alternative')
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
    it('shows dropdown even when siblingAgentIds is empty', () => {
      renderForm({ siblingAgentIds: [] })
      expect(screen.getByText('Low consumption alternative')).toBeInTheDocument()
    })

    it('shows dropdown when siblingAgentIds is omitted', () => {
      renderForm()
      expect(screen.getByText('Low consumption alternative')).toBeInTheDocument()
    })

    it('shows helper text', () => {
      renderForm()
      expect(screen.getByText(/agent to substitute when low consumption mode is enabled/i)).toBeInTheDocument()
    })
  })

  describe('options', () => {
    it('shows (none) plus sibling options', async () => {
      const user = userEvent.setup()
      renderForm({ siblingAgentIds: ['haiku-agent', 'fast-agent'] })
      await user.click(getLCDropdownButton())
      expect(screen.getAllByText('(none)').length).toBeGreaterThanOrEqual(1)
      expect(screen.getByText('haiku-agent')).toBeInTheDocument()
      expect(screen.getByText('fast-agent')).toBeInTheDocument()
    })

    it('defaults to (none) when no initial value', () => {
      renderForm({ siblingAgentIds: ['haiku-agent'] })
      expect(getLCDropdownButton().textContent).toContain('(none)')
    })
  })

  describe('selection and submission', () => {
    it('submits selected sibling as low_consumption_agent', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit, siblingAgentIds: ['haiku-agent', 'fast-agent'] })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Test prompt')
      await selectDropdownOption(user, getLCDropdownButton(), 'haiku-agent')
      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ low_consumption_agent: 'haiku-agent' })
      )
    })

    it('submits undefined when (none) selected', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit, siblingAgentIds: ['haiku-agent'] })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Test prompt')
      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ low_consumption_agent: undefined })
      )
    })

    it('pre-selects initial low_consumption_agent', () => {
      renderForm({
        isCreate: false,
        initial: { low_consumption_agent: 'haiku-agent' },
        siblingAgentIds: ['haiku-agent', 'fast-agent'],
      })
      expect(getLCDropdownButton().textContent).toContain('haiku-agent')
    })

    it('allows clearing back to (none) in update mode', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { low_consumption_agent: 'haiku-agent', prompt: 'Test' },
        onSubmit,
        siblingAgentIds: ['haiku-agent'],
      })

      expect(getLCDropdownButton().textContent).toContain('haiku-agent')
      await selectDropdownOption(user, getLCDropdownButton(), '(none)')
      await user.click(screen.getByRole('button', { name: /^save$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ low_consumption_agent: undefined })
      )
    })
  })
})
