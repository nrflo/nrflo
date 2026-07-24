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
      { value: 'haiku-4-5', label: 'Anthropic: Haiku' },
      { value: 'opus-4-8', label: 'Anthropic: Opus' },
      { value: 'opus-4-8-1m', label: 'Anthropic: Opus 1M' },
      { value: 'sonnet-5', label: 'Anthropic: Sonnet' },
    ]},
    { label: 'OpenAI', options: [
      { value: 'gpt-5.3-codex', label: 'OpenAI: GPT 5.3 Codex' },
      { value: 'gpt-5.4', label: 'OpenAI: GPT 5.4' },
    ]},
  ],
  useModels: () => ({ data: [] }),
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
    ...renderWithQuery(<AgentDefForm {...defaultProps} />),
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

/** Toggle "Override model (skip tier fallback chain)" on, exposing the Model dropdown */
async function enableOverride(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('switch', { name: /override model/i }))
}

function getTimeoutInput() {
  return screen.getAllByRole('spinbutton').find(el => (el as HTMLInputElement).min === '1' && !((el as HTMLInputElement).max)) as HTMLInputElement
}

function getRestartInput() {
  return screen.getAllByRole('spinbutton').find(el => (el as HTMLInputElement).max === '99') as HTMLInputElement
}

describe('AgentDefForm', () => {
  describe('form submission', () => {
    it('submits create request with all fields', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })
      await enableOverride(user)

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'setup-analyzer')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'You are a setup analyzer...')

      await selectDropdownOption(user, getModelDropdownButton(), 'Anthropic: Opus')

      const timeoutInput = getTimeoutInput()
      await user.clear(timeoutInput)
      await user.type(timeoutInput, '30')

      const restartInput = getRestartInput()
      await user.type(restartInput, '20')

      const submitButton = screen.getByRole('button', { name: /create/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith({
        id: 'setup-analyzer',
        layer: 0,
        model: 'opus-4-8',
        tier: null,
        timeout: 30,
        prompt: 'You are a setup analyzer...',
        restart_threshold: 20,
        max_fail_restarts: undefined,
        tag: undefined,
        low_consumption_model: undefined,
        execution_mode: 'cli_interactive',
        tools: '', native_tools: '', sandbox: '',
        api_max_iterations: undefined, api_max_tokens: undefined,
        validation_commands: [],
        consultant: undefined, node_role: undefined, description: undefined,
        reasoning_effort: null, system_template_id: undefined,
        prompt_mode: 'full',
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
        layer: 0,
        model: '',
        tier: 1,
        timeout: 20,
        prompt: 'New prompt',
        restart_threshold: undefined,
        max_fail_restarts: undefined,
        tag: undefined,
        low_consumption_model: undefined,
        execution_mode: 'cli_interactive',
        tools: '', native_tools: '', sandbox: '',
        api_max_iterations: undefined, api_max_tokens: undefined,
        validation_commands: [],
        consultant: undefined, node_role: undefined, description: undefined,
        reasoning_effort: null, system_template_id: '',
        prompt_mode: 'full',
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
          model: 'haiku-4-5',
          timeout: 45,
          restart_threshold: 30,
          prompt: 'Initial prompt',
        },
      })

      expect(getModelDropdownButton().textContent).toContain('Anthropic: Haiku')
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
})
