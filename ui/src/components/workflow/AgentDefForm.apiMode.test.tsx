import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

vi.mock('@/hooks/useCLIModels', () => ({
  useModelOptions: () => [
    { label: 'Claude', options: [{ value: 'sonnet', label: 'Claude: Sonnet' }] },
  ],
  useCLIModels: () => ({ data: [] }),
}))

vi.mock('@/hooks/useAPIModels', () => ({ useAPIModelOptions: () => [] }))

const mockUseAPIModeEnabled = vi.fn().mockReturnValue(true)
vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => mockUseAPIModeEnabled(),
}))


vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder?: string }) => (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      aria-label="Prompt Template"
    />
  ),
}))

function getExecutionModeButton() {
  const label = screen.getByText('Execution Mode')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

function renderForm(props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}) {
  return render(
    <AgentDefForm
      isCreate={true}
      onSubmit={vi.fn()}
      onCancel={vi.fn()}
      {...props}
    />
  )
}

// The tools picker itself is covered by AgentDefToolsField.test.tsx; here we
// only assert it is mounted ("All tools (*)" toggle) and that API-only fields
// (max iterations/tokens) appear for api mode.
describe('AgentDefForm — execution mode', () => {
  describe('default CLI Interactive mode', () => {
    it('defaults to CLI Interactive mode', () => {
      renderForm()
      expect(getExecutionModeButton().textContent).toContain('CLI Interactive (PTY)')
    })

    it('shows the tools picker but not API-only fields in cli mode', () => {
      renderForm()
      expect(screen.getByText('All tools (*)')).toBeInTheDocument()
      expect(screen.queryByPlaceholderText('50')).not.toBeInTheDocument()
    })

    it('uses initial execution_mode when provided', () => {
      renderForm({ isCreate: false, initial: { execution_mode: 'api', tools: 'findings_add' } })
      expect(getExecutionModeButton().textContent).toContain('API')
      expect(screen.getByPlaceholderText('50')).toBeInTheDocument()
    })
  })

  describe('switching to API mode', () => {
    it('shows API max fields after switching to api', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))

      expect(screen.getByPlaceholderText('50')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('16384')).toBeInTheDocument()
    })

    it('keeps the tools picker visible across cli and api', async () => {
      const user = userEvent.setup()
      renderForm()

      expect(screen.getByText('All tools (*)')).toBeInTheDocument()
      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))
      expect(screen.getByText('All tools (*)')).toBeInTheDocument()
    })

    it('hides API max fields when switching back to cli', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))
      expect(screen.getByPlaceholderText('50')).toBeInTheDocument()

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('CLI Interactive (PTY)'))
      expect(screen.queryByPlaceholderText('50')).not.toBeInTheDocument()
    })
  })

  describe('form submission with API mode', () => {
    it('includes execution_mode=api in payload', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'api-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'You are an API agent')

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))

      await user.click(screen.getByRole('button', { name: /create/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ execution_mode: 'api' })
      )
    })

    it('includes api_max_iterations when set', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'api-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'prompt')

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))
      await user.type(screen.getByPlaceholderText('50'), '25')

      await user.click(screen.getByRole('button', { name: /create/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ execution_mode: 'api', api_max_iterations: 25 })
      )
    })

    it('includes api_max_tokens when set', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'api-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'prompt')

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))
      await user.type(screen.getByPlaceholderText('16384'), '32768')

      await user.click(screen.getByRole('button', { name: /create/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ execution_mode: 'api', api_max_tokens: 32768 })
      )
    })

    it('api_max_iterations is undefined when not set', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'api-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'prompt')

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))

      await user.click(screen.getByRole('button', { name: /create/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ api_max_iterations: undefined })
      )
    })

    it('cli_interactive submit includes execution_mode=cli_interactive and empty tools', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'cli-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'prompt')

      await user.click(screen.getByRole('button', { name: /create/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ execution_mode: 'cli_interactive', tools: '' })
      )
    })
  })
})

describe('AgentDefForm — apiModeEnabled=false gate', () => {
  beforeEach(() => {
    mockUseAPIModeEnabled.mockReturnValue(false)
  })

  afterEach(() => {
    mockUseAPIModeEnabled.mockReturnValue(true)
  })

  it('hides API option from Execution Mode dropdown', async () => {
    const user = userEvent.setup()
    renderForm()
    expect(screen.getByText('Execution Mode')).toBeInTheDocument()
    await user.click(getExecutionModeButton())
    expect(screen.queryByText('API (in-process Anthropic runner)')).not.toBeInTheDocument()
    expect(screen.getByText('Script (Python)')).toBeInTheDocument()
  })

  it('cli_interactive submit still produces execution_mode=cli_interactive and tools=empty', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ onSubmit })

    await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'cli-agent')
    await user.type(screen.getByLabelText('Prompt Template'), 'prompt')
    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ execution_mode: 'cli_interactive', tools: '' })
    )
  })

  it('shows API max fields for an orphan api def even without the API option', () => {
    renderForm({ isCreate: false, initial: { execution_mode: 'api', tools: 'findings_add' } })
    expect(screen.getByText('Execution Mode')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('50')).toBeInTheDocument()
  })
})
