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

function getLayerInput() {
  return screen.getAllByRole('spinbutton').find(el => (el as HTMLInputElement).min === '0' && !((el as HTMLInputElement).max)) as HTMLInputElement
}

function getTimeoutInput() {
  return screen.getAllByRole('spinbutton').find(el => (el as HTMLInputElement).min === '1' && !((el as HTMLInputElement).max)) as HTMLInputElement
}

describe('AgentDefForm layer field and edge cases', () => {
  describe('layer field', () => {
    it('renders with default value 0 in create mode', () => {
      renderForm({ isCreate: true })
      const layerInput = getLayerInput()
      expect(layerInput).toBeInTheDocument()
      expect(layerInput).toHaveValue(0)
      expect(layerInput.type).toBe('number')
    })

    it('populates from initial layer value in edit mode', () => {
      renderForm({
        isCreate: false,
        initial: { layer: 3, prompt: 'Test' },
      })
      expect(getLayerInput()).toHaveValue(3)
    })

    it('includes changed layer in create payload', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'test-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Prompt')

      const layerInput = getLayerInput()
      await user.clear(layerInput)
      await user.type(layerInput, '2')

      await user.click(screen.getByRole('button', { name: /create/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'test-agent', layer: 2 })
      )
    })

    it('includes changed layer in update payload', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { layer: 1, prompt: 'Test' },
        onSubmit,
      })

      const layerInput = getLayerInput()
      await user.clear(layerInput)
      await user.type(layerInput, '5')

      await user.click(screen.getByRole('button', { name: /save/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ layer: 5 })
      )
      // Update payload should not include id
      expect(onSubmit).toHaveBeenCalledWith(
        expect.not.objectContaining({ id: expect.anything() })
      )
    })

    it('shows help text about execution order', () => {
      renderForm({ isCreate: true })
      expect(screen.getByText(/layer 0 runs first/i)).toBeInTheDocument()
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

      const restartInput = screen.getAllByRole('spinbutton').find(el => (el as HTMLInputElement).max === '99') as HTMLInputElement
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

      const restartInput = screen.getAllByRole('spinbutton').find(el => (el as HTMLInputElement).max === '99') as HTMLInputElement
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

  describe('execution mode parity across providers', () => {
    function getExecutionModeButton() {
      return screen.getByText('Execution Mode')
        .parentElement!
        .querySelector('button[type="button"]') as HTMLButtonElement
    }

    const expectedModes = [
      'CLI Interactive (PTY)',
      'API (in-process Anthropic runner)',
      'Script (Python)',
    ]

    it.each([
      ['claude', 'sonnet-5'],
      ['codex', 'gpt-5.3-codex'],
    ])('shows same execution mode options for %s (%s)', async (_, model) => {
      const user = userEvent.setup()
      renderForm({ isCreate: false, initial: { model, prompt: 'test' } })

      expect(getExecutionModeButton().textContent).toContain('CLI Interactive (PTY)')

      await user.click(getExecutionModeButton())
      const optionsContainer = getExecutionModeButton().parentElement!.querySelector('.absolute')!
      const optionLabels = Array.from(optionsContainer.querySelectorAll('span.truncate')).map(el => el.textContent)
      expect(optionLabels).toEqual(expectedModes)
    })
  })
})
