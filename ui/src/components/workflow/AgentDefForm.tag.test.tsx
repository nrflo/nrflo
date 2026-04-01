import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

vi.mock('@/hooks/useCLIModels', () => ({
  useModelOptions: () => [
    { value: 'sonnet', label: 'sonnet' },
    { value: 'opus', label: 'opus' },
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

function renderForm(
  props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}
) {
  const defaultProps = {
    isCreate: true,
    onSubmit: vi.fn(),
    onCancel: vi.fn(),
    ...props,
  }
  return {
    ...render(<AgentDefForm {...defaultProps} />),
    props: defaultProps,
  }
}

function getTagDropdownButton() {
  const label = screen.getByText('Tag')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

async function selectDropdownOption(
  user: ReturnType<typeof userEvent.setup>,
  triggerButton: HTMLButtonElement,
  optionLabel: string
) {
  await user.click(triggerButton)
  // Scope to the dropdown panel (sibling of trigger button) to avoid matching other dropdowns
  const dropdownContainer = triggerButton.closest('.relative')!
  const option = Array.from(dropdownContainer.querySelectorAll('.cursor-pointer span')).find(
    (el) => el.textContent === optionLabel
  ) as HTMLElement
  await user.click(option)
}

describe('AgentDefForm - tag dropdown', () => {
  describe('visibility', () => {
    it('does not show tag dropdown when groups is empty', () => {
      renderForm({ groups: [] })
      expect(screen.queryByText('Tag')).not.toBeInTheDocument()
    })

    it('does not show tag dropdown when groups prop is omitted', () => {
      renderForm()
      expect(screen.queryByText('Tag')).not.toBeInTheDocument()
    })

    it('shows tag dropdown when groups has items', () => {
      renderForm({ groups: ['be', 'fe'] })
      expect(screen.getByText('Tag')).toBeInTheDocument()
    })

    it('shows helper text when groups present', () => {
      renderForm({ groups: ['be'] })
      expect(screen.getByText(/assign a group tag for skip logic/i)).toBeInTheDocument()
    })

    it('does not show helper text when groups empty', () => {
      renderForm({ groups: [] })
      expect(screen.queryByText(/assign a group tag for skip logic/i)).not.toBeInTheDocument()
    })
  })

  describe('options', () => {
    it('shows (none) option plus all group options', async () => {
      const user = userEvent.setup()
      renderForm({ groups: ['be', 'fe', 'docs'] })

      await user.click(getTagDropdownButton())

      // Dropdown shows (none) in both trigger and open list — use getAllByText
      expect(screen.getAllByText('(none)').length).toBeGreaterThanOrEqual(1)
      expect(screen.getByText('be')).toBeInTheDocument()
      expect(screen.getByText('fe')).toBeInTheDocument()
      expect(screen.getByText('docs')).toBeInTheDocument()
    })

    it('defaults to (none) when no initial tag', () => {
      renderForm({ groups: ['be', 'fe'] })
      expect(getTagDropdownButton().textContent).toContain('(none)')
    })
  })

  describe('selection and submission', () => {
    it('submits selected tag value', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit, groups: ['be', 'fe'] })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Test prompt')
      await selectDropdownOption(user, getTagDropdownButton(), 'fe')

      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ tag: 'fe' })
      )
    })

    it('submits undefined when (none) is selected', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit, groups: ['be', 'fe'] })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Test prompt')
      // (none) is already the default

      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ tag: undefined })
      )
    })

    it('pre-selects initial tag', () => {
      renderForm({
        isCreate: false,
        initial: { tag: 'be' },
        groups: ['be', 'fe'],
      })

      expect(getTagDropdownButton().textContent).toContain('be')
    })

    it('allows clearing tag back to (none) in update mode', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { tag: 'be', prompt: 'Test' },
        onSubmit,
        groups: ['be', 'fe'],
      })

      expect(getTagDropdownButton().textContent).toContain('be')

      await selectDropdownOption(user, getTagDropdownButton(), '(none)')

      await user.click(screen.getByRole('button', { name: /^save$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ tag: undefined })
      )
    })
  })
})
