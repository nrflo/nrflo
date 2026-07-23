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

describe('AgentDefForm model dropdown', () => {
  describe('model dropdown', () => {
    it('renders every mode-supported model option', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })
      await enableOverride(user)

      const dropdownBtn = getModelDropdownButton()
      expect(dropdownBtn).toBeInTheDocument()

      // Open the dropdown to see options
      await user.click(dropdownBtn)

      // Each option is rendered as a div with the label text inside the dropdown menu
      const optionsContainer = dropdownBtn.parentElement!.querySelector('.absolute')!
      const optionDivs = optionsContainer.querySelectorAll('.cursor-pointer')
      expect(optionDivs).toHaveLength(6)
    })

    it('contains all model options', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })
      await enableOverride(user)

      await user.click(getModelDropdownButton())

      const optionsContainer = getModelDropdownButton().parentElement!.querySelector('.absolute')!
      const optionTexts = Array.from(optionsContainer.querySelectorAll('.truncate')).map(el => el.textContent)
      expect(optionTexts).toEqual(['Anthropic: Haiku', 'Anthropic: Opus', 'Anthropic: Opus 1M', 'Anthropic: Sonnet', 'OpenAI: GPT 5.3 Codex', 'OpenAI: GPT 5.4'])
    })

    it('defaults to sonnet', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })
      await enableOverride(user)

      const dropdownBtn = getModelDropdownButton()
      expect(dropdownBtn.textContent).toContain('Anthropic: Sonnet')
    })

    it('uses initial model value when provided', () => {
      renderForm({
        isCreate: false,
        initial: { model: 'opus-4-8' },
      })

      const dropdownBtn = getModelDropdownButton()
      expect(dropdownBtn.textContent).toContain('Anthropic: Opus')
    })

    it('allows changing model selection', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })
      await enableOverride(user)

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'test-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'Test prompt')

      await selectDropdownOption(user, getModelDropdownButton(), 'OpenAI: GPT 5.3 Codex')

      const submitButton = screen.getByRole('button', { name: /create/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          model: 'gpt-5.3-codex',
        })
      )
    })

    it('model dropdown uses correct styling', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })
      await enableOverride(user)

      const dropdownBtn = getModelDropdownButton()
      expect(dropdownBtn.className).toContain('rounded-md')
      expect(dropdownBtn.className).toContain('border')
      expect(dropdownBtn.className).toContain('text-sm')
    })
  })

  describe('model dropdown options validation', () => {
    it('opus option exists and is selectable', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })
      await enableOverride(user)

      await selectDropdownOption(user, getModelDropdownButton(), 'Anthropic: Opus')

      expect(getModelDropdownButton().textContent).toContain('Anthropic: Opus')
    })

    it('sonnet option exists and is selectable', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })
      await enableOverride(user)

      // sonnet is the default, so it's already selected
      expect(getModelDropdownButton().textContent).toContain('Anthropic: Sonnet')
    })

    it('haiku option exists and is selectable', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })
      await enableOverride(user)

      await selectDropdownOption(user, getModelDropdownButton(), 'Anthropic: Haiku')

      expect(getModelDropdownButton().textContent).toContain('Anthropic: Haiku')
    })

    it('no extra model options exist', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })
      await enableOverride(user)

      // Open dropdown to see options
      await user.click(getModelDropdownButton())

      const optionsContainer = getModelDropdownButton().parentElement!.querySelector('.absolute')!
      const optionTexts = Array.from(optionsContainer.querySelectorAll('.truncate')).map(el => el.textContent)

      expect(optionTexts).toHaveLength(6)
      expect(optionTexts).toEqual(['Anthropic: Haiku', 'Anthropic: Opus', 'Anthropic: Opus 1M', 'Anthropic: Sonnet', 'OpenAI: GPT 5.3 Codex', 'OpenAI: GPT 5.4'])
    })
  })
})
