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

describe('AgentDefForm — execution mode', () => {
  describe('default CLI mode', () => {
    it('defaults to CLI mode', () => {
      renderForm()
      expect(getExecutionModeButton().textContent).toContain('CLI (default)')
    })

    it('does not show API fields in cli mode', () => {
      renderForm()
      expect(screen.queryByPlaceholderText(/findings_add/i)).not.toBeInTheDocument()
      expect(screen.queryByPlaceholderText('50')).not.toBeInTheDocument()
    })

    it('uses initial execution_mode when provided', () => {
      renderForm({ isCreate: false, initial: { execution_mode: 'api', tools: 'findings_add' } })
      expect(getExecutionModeButton().textContent).toContain('API')
      expect(screen.getByPlaceholderText(/findings_add/i)).toBeInTheDocument()
    })
  })

  describe('switching to API mode', () => {
    it('shows tools input and max iterations after switching to api', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))

      expect(screen.getByPlaceholderText(/findings_add/i)).toBeInTheDocument()
      expect(screen.getByPlaceholderText('50')).toBeInTheDocument()
    })

    it('shows tools-empty warning immediately when switching to api with empty tools', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))

      expect(screen.getByText(/Tools must be non-empty/i)).toBeInTheDocument()
    })

    it('hides tools-empty warning when tools is filled in', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))

      const toolsInput = screen.getByPlaceholderText(/findings_add/i)
      await user.type(toolsInput, '*')

      expect(screen.queryByText(/Tools must be non-empty/i)).not.toBeInTheDocument()
    })

    it('hides API fields when switching back to cli', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))

      expect(screen.getByPlaceholderText(/findings_add/i)).toBeInTheDocument()

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('CLI (default)'))

      expect(screen.queryByPlaceholderText(/findings_add/i)).not.toBeInTheDocument()
    })
  })

  describe('form submission with API mode', () => {
    it('includes execution_mode=api and tools in payload', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'api-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'You are an API agent')

      await user.click(getExecutionModeButton())
      await user.click(screen.getByText('API (in-process Anthropic runner)'))

      const toolsInput = screen.getByPlaceholderText(/findings_add/i)
      await user.type(toolsInput, 'findings_add,agent_fail')

      await user.click(screen.getByRole('button', { name: /create/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          execution_mode: 'api',
          tools: 'findings_add,agent_fail',
        })
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

      await user.type(screen.getByPlaceholderText(/findings_add/i), '*')
      await user.type(screen.getByPlaceholderText('50'), '25')

      await user.click(screen.getByRole('button', { name: /create/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          execution_mode: 'api',
          api_max_iterations: 25,
        })
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
      await user.type(screen.getByPlaceholderText(/findings_add/i), '*')

      await user.click(screen.getByRole('button', { name: /create/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ api_max_iterations: undefined })
      )
    })

    it('cli submit includes execution_mode=cli and empty tools', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'cli-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'prompt')

      await user.click(screen.getByRole('button', { name: /create/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ execution_mode: 'cli', tools: '' })
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
    // Execution Mode dropdown is always visible (controls cli/api/script)
    expect(screen.getByText('Execution Mode')).toBeInTheDocument()
    // Open dropdown — API option must not appear; Script option must be available
    await user.click(getExecutionModeButton())
    expect(screen.queryByText('API (in-process Anthropic runner)')).not.toBeInTheDocument()
    expect(screen.getByText('Script (Python)')).toBeInTheDocument()
  })

  it('cli submit still produces execution_mode=cli and tools=empty', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ onSubmit })

    await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'cli-agent')
    await user.type(screen.getByLabelText('Prompt Template'), 'prompt')
    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ execution_mode: 'cli', tools: '' })
    )
  })

  it('shows AgentDefAPIModeFields for orphan api def even without API option in dropdown', () => {
    renderForm({ isCreate: false, initial: { execution_mode: 'api', tools: 'findings_add' } })
    // Dropdown always visible; API option absent but existing api-mode def fields still rendered
    expect(screen.getByText('Execution Mode')).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/findings_add/i)).toBeInTheDocument()
  })
})
