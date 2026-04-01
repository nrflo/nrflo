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

// Mock MarkdownEditor to avoid CodeMirror dependencies
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

/** Get the Dropdown trigger button for the model field */
function getModelDropdownButton() {
  // The Model label is followed by the Dropdown which renders a <button type="button">
  const label = screen.getByText('Model')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

/** Select an option from the Dropdown by opening it and clicking the option */
async function selectDropdownOption(user: ReturnType<typeof userEvent.setup>, triggerButton: HTMLButtonElement, optionLabel: string) {
  await user.click(triggerButton)
  await user.click(screen.getByText(optionLabel))
}

function getTimeoutInput() {
  return screen.getAllByRole('spinbutton').find(el => (el as HTMLInputElement).min === '1' && !((el as HTMLInputElement).max)) as HTMLInputElement
}

function getRestartInput() {
  return screen.getAllByRole('spinbutton').find(el => (el as HTMLInputElement).max === '99') as HTMLInputElement
}

describe('AgentDefForm', () => {
  describe('model dropdown', () => {
    it('renders model dropdown with exactly 9 options', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      const dropdownBtn = getModelDropdownButton()
      expect(dropdownBtn).toBeInTheDocument()

      // Open the dropdown to see options
      await user.click(dropdownBtn)

      // Each option is rendered as a div with the label text inside the dropdown menu
      const optionsContainer = dropdownBtn.parentElement!.querySelector('.absolute')!
      const optionDivs = optionsContainer.querySelectorAll('.cursor-pointer')
      expect(optionDivs).toHaveLength(10)
    })

    it('contains all model options', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      await user.click(getModelDropdownButton())

      const optionsContainer = getModelDropdownButton().parentElement!.querySelector('.absolute')!
      const optionTexts = Array.from(optionsContainer.querySelectorAll('.truncate')).map(el => el.textContent)
      expect(optionTexts).toEqual(['Claude: Haiku', 'Claude: Opus', 'Claude: Opus 1M', 'Claude: Sonnet', 'Codex: GPT (High)', 'Codex: GPT (Normal)', 'Codex: GPT-54 (High)', 'Codex: GPT-54 (Normal)', 'OpenCode: GPT (High)', 'OpenCode: GPT (Normal)'])
    })

    it('defaults to sonnet', () => {
      renderForm({ isCreate: true })

      const dropdownBtn = getModelDropdownButton()
      expect(dropdownBtn.textContent).toContain('Claude: Sonnet')
    })

    it('uses initial model value when provided', () => {
      renderForm({
        isCreate: false,
        initial: { model: 'opus' },
      })

      const dropdownBtn = getModelDropdownButton()
      expect(dropdownBtn.textContent).toContain('Claude: Opus')
    })

    it('allows changing model selection', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'test-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Test prompt')

      await selectDropdownOption(user, getModelDropdownButton(), 'OpenCode: GPT (High)')

      const submitButton = screen.getByRole('button', { name: /create/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          model: 'opencode_gpt_high',
        })
      )
    })

    it('model dropdown uses correct styling', () => {
      renderForm({ isCreate: true })

      const dropdownBtn = getModelDropdownButton()
      expect(dropdownBtn.className).toContain('rounded-md')
      expect(dropdownBtn.className).toContain('border')
      expect(dropdownBtn.className).toContain('text-sm')
    })
  })

  describe('form submission', () => {
    it('submits create request with all fields', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'setup-analyzer')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'You are a setup analyzer...')

      await selectDropdownOption(user, getModelDropdownButton(), 'Claude: Opus')

      const timeoutInput = getTimeoutInput()
      await user.clear(timeoutInput)
      await user.type(timeoutInput, '30')

      const restartInput = getRestartInput()
      await user.type(restartInput, '20')

      const submitButton = screen.getByRole('button', { name: /create/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith({
        id: 'setup-analyzer',
        model: 'opus',
        timeout: 30,
        prompt: 'You are a setup analyzer...',
        restart_threshold: 20,
        tag: undefined,
      })
    })

    it('submits update request without id', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { id: 'setup-analyzer', prompt: 'Old prompt' },
        onSubmit,
      })

      const promptInput = screen.getByPlaceholderText(/agent prompt template/i)
      await user.clear(promptInput)
      await user.type(promptInput, 'New prompt')

      const submitButton = screen.getByRole('button', { name: /save/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith({
        model: 'sonnet',
        timeout: 20,
        prompt: 'New prompt',
        restart_threshold: undefined,
        tag: undefined,
      })
    })

    it('does not submit when prompt is empty in create mode', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'test-agent')
      // Leave prompt empty

      const submitButton = screen.getByRole('button', { name: /create/i })
      await user.click(submitButton)

      expect(onSubmit).not.toHaveBeenCalled()
    })

    it('handles empty restart_threshold (undefined)', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'test-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Test prompt')
      // Leave restart_threshold empty

      const submitButton = screen.getByRole('button', { name: /create/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: undefined,
        })
      )
    })

    it('includes restart_threshold when provided', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'test-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Test prompt')

      const restartInput = getRestartInput()
      await user.type(restartInput, '15')

      const submitButton = screen.getByRole('button', { name: /create/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: 15,
        })
      )
    })
  })

  describe('form fields', () => {
    it('shows ID field only in create mode', () => {
      const { rerender } = renderForm({ isCreate: true })
      expect(screen.getByPlaceholderText(/e.g., setup-analyzer/i)).toBeInTheDocument()

      rerender(<AgentDefForm isCreate={false} onSubmit={vi.fn()} onCancel={vi.fn()} />)
      expect(screen.queryByPlaceholderText(/e.g., setup-analyzer/i)).not.toBeInTheDocument()
    })

    it('renders timeout field with default value 20', () => {
      renderForm({ isCreate: true })

      const timeoutInput = getTimeoutInput()
      expect(timeoutInput).toBeInTheDocument()
      expect(timeoutInput.value).toBe('20')
      expect(timeoutInput.type).toBe('number')
    })

    it('renders restart threshold field with placeholder', () => {
      renderForm({ isCreate: true })

      const restartInput = getRestartInput()
      expect(restartInput).toBeInTheDocument()
      expect(restartInput.placeholder).toBe('25')
      expect(restartInput.type).toBe('number')
    })

    it('uses initial values when provided', () => {
      renderForm({
        isCreate: false,
        initial: {
          id: 'test-agent',
          model: 'haiku',
          timeout: 45,
          restart_threshold: 30,
          prompt: 'Initial prompt',
        },
      })

      expect(getModelDropdownButton().textContent).toContain('Claude: Haiku')
      expect(getTimeoutInput()).toHaveValue(45)
      expect(getRestartInput()).toHaveValue(30)
      expect(screen.getByPlaceholderText(/agent prompt template/i)).toHaveValue('Initial prompt')
    })
  })

  describe('form actions', () => {
    it('calls onCancel when cancel button clicked', async () => {
      const user = userEvent.setup()
      const onCancel = vi.fn()
      renderForm({ onCancel })

      const cancelButton = screen.getByRole('button', { name: /cancel/i })
      await user.click(cancelButton)

      expect(onCancel).toHaveBeenCalledTimes(1)
    })

    it('shows correct button text based on mode', () => {
      const { rerender } = renderForm({ isCreate: true })
      expect(screen.getByRole('button', { name: /^create$/i })).toBeInTheDocument()

      rerender(<AgentDefForm isCreate={false} onSubmit={vi.fn()} onCancel={vi.fn()} />)
      expect(screen.getByRole('button', { name: /^save$/i })).toBeInTheDocument()
    })
  })

  describe('model dropdown options validation', () => {
    it('opus option exists and is selectable', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      await selectDropdownOption(user, getModelDropdownButton(), 'Claude: Opus')

      expect(getModelDropdownButton().textContent).toContain('Claude: Opus')
    })

    it('sonnet option exists and is selectable', () => {
      renderForm({ isCreate: true })

      // sonnet is the default, so it's already selected
      expect(getModelDropdownButton().textContent).toContain('Claude: Sonnet')
    })

    it('haiku option exists and is selectable', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      await selectDropdownOption(user, getModelDropdownButton(), 'Claude: Haiku')

      expect(getModelDropdownButton().textContent).toContain('Claude: Haiku')
    })

    it('opencode_gpt_high option exists and is selectable', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      await selectDropdownOption(user, getModelDropdownButton(), 'OpenCode: GPT (High)')

      expect(getModelDropdownButton().textContent).toContain('OpenCode: GPT (High)')
    })

    it('no extra model options exist', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      // Open dropdown to see options
      await user.click(getModelDropdownButton())

      const optionsContainer = getModelDropdownButton().parentElement!.querySelector('.absolute')!
      const optionTexts = Array.from(optionsContainer.querySelectorAll('.truncate')).map(el => el.textContent)

      expect(optionTexts).toHaveLength(10)
      expect(optionTexts).toEqual(['Claude: Haiku', 'Claude: Opus', 'Claude: Opus 1M', 'Claude: Sonnet', 'Codex: GPT (High)', 'Codex: GPT (Normal)', 'Codex: GPT-54 (High)', 'Codex: GPT-54 (Normal)', 'OpenCode: GPT (High)', 'OpenCode: GPT (Normal)'])
    })
  })

  describe('edge cases', () => {
    it('handles changing timeout to minimum value', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'test')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Prompt')

      const timeoutInput = getTimeoutInput()
      await user.clear(timeoutInput)
      await user.type(timeoutInput, '1')

      const submitButton = screen.getByRole('button', { name: /create/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          timeout: 1,
        })
      )
    })

    it('handles restart_threshold at boundaries', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'test')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Prompt')

      const restartInput = getRestartInput()
      await user.type(restartInput, '99')

      const submitButton = screen.getByRole('button', { name: /create/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: 99,
        })
      )
    })

    it('handles clearing restart_threshold after setting value', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { restart_threshold: 25, prompt: 'Test' },
        onSubmit,
      })

      const restartInput = getRestartInput()
      await user.clear(restartInput)

      const submitButton = screen.getByRole('button', { name: /save/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          restart_threshold: undefined,
        })
      )
    })
  })
})
