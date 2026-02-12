import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

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

function getModelSelect() {
  return document.querySelector('select') as HTMLSelectElement
}

function getTimeoutInput() {
  return screen.getAllByRole('spinbutton').find(el => (el as HTMLInputElement).min === '1' && !((el as HTMLInputElement).max)) as HTMLInputElement
}

function getRestartInput() {
  return screen.getAllByRole('spinbutton').find(el => (el as HTMLInputElement).max === '99') as HTMLInputElement
}

describe('AgentDefForm', () => {
  describe('model dropdown', () => {
    it('renders model dropdown with exactly 4 options', () => {
      renderForm({ isCreate: true })

      const modelSelect = getModelSelect()
      expect(modelSelect).toBeInTheDocument()

      const options = Array.from(modelSelect.querySelectorAll('option'))
      expect(options).toHaveLength(4)
    })

    it('contains opus, sonnet, haiku, gpt_5.3 options', () => {
      renderForm({ isCreate: true })

      const modelSelect = getModelSelect()
      const options = Array.from(modelSelect.querySelectorAll('option'))
      const values = options.map((opt) => opt.value)

      expect(values).toEqual(['opus', 'sonnet', 'haiku', 'gpt_5.3'])
    })

    it('defaults to sonnet', () => {
      renderForm({ isCreate: true })

      const modelSelect = getModelSelect()
      expect(modelSelect.value).toBe('sonnet')
    })

    it('uses initial model value when provided', () => {
      renderForm({
        isCreate: false,
        initial: { model: 'opus' },
      })

      const modelSelect = getModelSelect()
      expect(modelSelect.value).toBe('opus')
    })

    it('allows changing model selection', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'test-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Test prompt')

      const modelSelect = getModelSelect()
      await user.selectOptions(modelSelect, 'gpt_5.3')

      const submitButton = screen.getByRole('button', { name: /create/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          model: 'gpt_5.3',
        })
      )
    })

    it('model dropdown uses correct styling', () => {
      renderForm({ isCreate: true })

      const modelSelect = getModelSelect()
      expect(modelSelect.className).toContain('rounded-md')
      expect(modelSelect.className).toContain('border')
      expect(modelSelect.className).toContain('bg-background')
      expect(modelSelect.className).toContain('px-3')
      expect(modelSelect.className).toContain('text-sm')
    })
  })

  describe('form submission', () => {
    it('submits create request with all fields', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'setup-analyzer')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'You are a setup analyzer...')

      const modelSelect = getModelSelect()
      await user.selectOptions(modelSelect, 'opus')

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

      expect(getModelSelect()).toHaveValue('haiku')
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

      const modelSelect = getModelSelect()
      await user.selectOptions(modelSelect, 'opus')

      expect(modelSelect).toHaveValue('opus')
    })

    it('sonnet option exists and is selectable', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      const modelSelect = getModelSelect()
      await user.selectOptions(modelSelect, 'sonnet')

      expect(modelSelect).toHaveValue('sonnet')
    })

    it('haiku option exists and is selectable', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      const modelSelect = getModelSelect()
      await user.selectOptions(modelSelect, 'haiku')

      expect(modelSelect).toHaveValue('haiku')
    })

    it('gpt_5.3 option exists and is selectable', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      const modelSelect = getModelSelect()
      await user.selectOptions(modelSelect, 'gpt_5.3')

      expect(modelSelect).toHaveValue('gpt_5.3')
    })

    it('no extra model options exist', () => {
      renderForm({ isCreate: true })

      const modelSelect = getModelSelect()
      const options = Array.from(modelSelect.querySelectorAll('option'))

      // Verify exactly 4 options with correct values
      expect(options).toHaveLength(4)
      const values = options.map((opt) => opt.value)
      expect(values).toContain('opus')
      expect(values).toContain('sonnet')
      expect(values).toContain('haiku')
      expect(values).toContain('gpt_5.3')

      // No other values
      expect(values.filter((v) => !['opus', 'sonnet', 'haiku', 'gpt_5.3'].includes(v))).toHaveLength(0)
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
