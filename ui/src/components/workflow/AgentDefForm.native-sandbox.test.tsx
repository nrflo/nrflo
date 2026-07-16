import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'
import type { AgentDefUpdateRequest } from '@/types/workflow'

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => true,
}))

vi.mock('@/hooks/useModels', () => ({
  useModelOptions: () => [
    { label: 'Anthropic', options: [{ value: 'sonnet-5', label: 'Anthropic: Sonnet' }] },
    { label: 'OpenAI', options: [{ value: 'gpt-5.4', label: 'OpenAI: GPT 5.4' }] },
  ],
  useModels: () => ({
    data: [
      { id: 'sonnet-5', provider: 'anthropic', enabled: true, cli_model: 'sonnet', display_name: 'Sonnet' },
      { id: 'gpt-5.4', provider: 'openai', enabled: true, cli_model: 'gpt-5.4', display_name: 'GPT 5.4' },
    ],
  }),
}))

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value, onChange, placeholder }: any) => (
    <textarea value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} aria-label="Prompt Template" />
  ),
}))

// AvailableTools query used by AgentDefToolsField — return nothing.
vi.mock('@/hooks/useAvailableTools', () => ({
  useAvailableTools: () => ({ data: [], isLoading: false }),
}))

// PythonScriptPickerField uses useQuery; no QueryClientProvider in this suite.
vi.mock('./PythonScriptPickerField', () => ({
  PythonScriptPickerField: ({ value }: { value: string }) => <div data-testid="script-picker">{value}</div>,
}))

function renderForm(props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}) {
  const defaultProps = { isCreate: false, onSubmit: vi.fn(), onCancel: vi.fn(), ...props }
  return { ...render(<AgentDefForm {...defaultProps} />), props: defaultProps }
}

describe('AgentDefForm native tools + sandbox', () => {
  it('anthropic cli agent shows native tools field, hides sandbox', () => {
    renderForm({ initial: { model: 'sonnet-5', execution_mode: 'cli_interactive' } })
    expect(screen.getByText('Native CLI tools (claude)')).toBeInTheDocument()
    expect(screen.queryByText('Sandbox (codex)')).not.toBeInTheDocument()
  })

  it('openai cli agent shows sandbox field, hides native tools', () => {
    renderForm({ initial: { model: 'gpt-5.4', execution_mode: 'cli_interactive' } })
    expect(screen.getByText('Sandbox (codex)')).toBeInTheDocument()
    expect(screen.queryByText('Native CLI tools (claude)')).not.toBeInTheDocument()
  })

  it('api mode hides both fields', () => {
    renderForm({ initial: { model: 'sonnet-5', execution_mode: 'api' } })
    expect(screen.queryByText('Native CLI tools (claude)')).not.toBeInTheDocument()
    expect(screen.queryByText('Sandbox (codex)')).not.toBeInTheDocument()
  })

  it('submit payload carries native_tools for an anthropic def', async () => {
    const user = userEvent.setup()
    const { props } = renderForm({
      initial: { model: 'sonnet-5', execution_mode: 'cli_interactive', native_tools: 'Read', prompt: 'p' },
    })
    await user.click(screen.getByRole('button', { name: 'Save' }))
    const payload = (props.onSubmit as ReturnType<typeof vi.fn>).mock.calls[0][0] as AgentDefUpdateRequest
    expect(payload.native_tools).toBe('Read')
    expect(payload.sandbox).toBe('')
  })

  it('switching model to another provider clears the stale restriction', async () => {
    const user = userEvent.setup()
    const { props } = renderForm({
      initial: { model: 'sonnet-5', execution_mode: 'cli_interactive', native_tools: 'Read', prompt: 'p' },
    })
    const modelLabel = screen.getByText('Model')
    const trigger = modelLabel.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    await user.click(trigger)
    await user.click(screen.getByText('OpenAI: GPT 5.4'))
    expect(screen.queryByText('Native CLI tools (claude)')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Save' }))
    const payload = (props.onSubmit as ReturnType<typeof vi.fn>).mock.calls[0][0] as AgentDefUpdateRequest
    expect(payload.model).toBe('gpt-5.4')
    expect(payload.native_tools).toBe('')
  })

  it('script mode omits both fields from the payload', async () => {
    const user = userEvent.setup()
    const { props } = renderForm({
      initial: { model: 'sonnet-5', execution_mode: 'script', python_script_id: 'ps1' },
    })
    await user.click(screen.getByRole('button', { name: 'Save' }))
    const payload = (props.onSubmit as ReturnType<typeof vi.fn>).mock.calls[0][0] as AgentDefUpdateRequest
    expect('native_tools' in payload).toBe(false)
    expect('sandbox' in payload).toBe(false)
  })
})
