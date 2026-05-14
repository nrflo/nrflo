import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

vi.mock('@/hooks/useCLIModels', () => ({
  useModelOptions: () => [],
  useCLIModels: () => ({ data: [] }),
}))

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => true,
}))

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value, onChange, placeholder }: {
    value: string; onChange: (v: string) => void; placeholder?: string
  }) => (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      aria-label="Prompt Template"
    />
  ),
}))

// Stub PythonScriptPickerField — avoids usePythonScripts/QueryClient requirement
vi.mock('@/components/workflow/PythonScriptPickerField', () => ({
  PythonScriptPickerField: ({ value, onChange }: { value: string; onChange: (v: string) => void }) => (
    <select aria-label="Python Script" value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">-- select script --</option>
      <option value="script-1">Script One</option>
      <option value="script-2">Script Two</option>
    </select>
  ),
}))

function getExecutionModeButton() {
  const label = screen.getByText('Execution Mode')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

function renderForm(props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}) {
  return render(
    <AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} {...props} />
  )
}

async function switchToScriptMode(user: ReturnType<typeof userEvent.setup>) {
  await user.click(getExecutionModeButton())
  await user.click(screen.getByText('Script (Python)'))
}

beforeEach(() => vi.clearAllMocks())

describe('AgentDefForm — script mode visibility', () => {
  it('includes Script (Python) in Execution Mode dropdown', async () => {
    const user = userEvent.setup()
    renderForm()
    await user.click(getExecutionModeButton())
    expect(screen.getByText('Script (Python)')).toBeInTheDocument()
  })

  it('hides Model label when script mode is selected', async () => {
    const user = userEvent.setup()
    renderForm()
    await switchToScriptMode(user)
    expect(screen.queryByText('Model')).not.toBeInTheDocument()
  })

  it('hides Prompt Template when script mode is selected', async () => {
    const user = userEvent.setup()
    renderForm()
    await switchToScriptMode(user)
    expect(screen.queryByText('Prompt Template')).not.toBeInTheDocument()
  })

  it('hides Low consumption model when script mode is selected', async () => {
    const user = userEvent.setup()
    renderForm()
    await switchToScriptMode(user)
    expect(screen.queryByText('Low consumption model')).not.toBeInTheDocument()
  })

  it('shows Python Script picker when script mode is selected', async () => {
    const user = userEvent.setup()
    renderForm()
    await switchToScriptMode(user)
    expect(screen.getByText(/Python Script/)).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: /python script/i })).toBeInTheDocument()
  })

  it('keeps Layer and Timeout fields visible in script mode', async () => {
    const user = userEvent.setup()
    renderForm()
    await switchToScriptMode(user)
    expect(screen.getByText('Layer')).toBeInTheDocument()
    expect(screen.getByText('Timeout (min)')).toBeInTheDocument()
  })
})

describe('AgentDefForm — script mode submission', () => {
  it('blocks submit when no script is selected', async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    renderForm({ onSubmit })
    await switchToScriptMode(user)
    await user.type(screen.getByPlaceholderText(/e\.g\., setup-analyzer/i), 'my-agent')
    await user.click(screen.getByRole('button', { name: /create/i }))
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('submits with python_script_id and execution_mode=script', async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    renderForm({ onSubmit })
    await switchToScriptMode(user)
    await user.type(screen.getByPlaceholderText(/e\.g\., setup-analyzer/i), 'my-agent')
    await user.selectOptions(screen.getByRole('combobox', { name: /python script/i }), 'script-1')
    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        execution_mode: 'script',
        python_script_id: 'script-1',
      })
    )
    // Must not include prompt, model, tools in payload
    const payload = onSubmit.mock.calls[0][0] as Record<string, unknown>
    expect(payload).not.toHaveProperty('prompt')
    expect(payload).not.toHaveProperty('model')
    expect(payload).not.toHaveProperty('tools')
    expect(payload).not.toHaveProperty('api_max_iterations')
  })

  it('clears python_script_id when switching from script back to cli_interactive', async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    renderForm({ onSubmit })

    // Switch to script, pick a script
    await switchToScriptMode(user)
    await user.selectOptions(screen.getByRole('combobox', { name: /python script/i }), 'script-2')

    // Switch back to CLI Interactive — python_script_id must be cleared
    await user.click(getExecutionModeButton())
    await user.click(screen.getByText('CLI Interactive (PTY)'))

    // Fill CLI-required fields and submit
    await user.type(screen.getByPlaceholderText(/e\.g\., setup-analyzer/i), 'my-agent')
    await user.type(screen.getByLabelText('Prompt Template'), 'some prompt')
    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ execution_mode: 'cli_interactive' })
    )
    const payload = onSubmit.mock.calls[0][0] as Record<string, unknown>
    expect(payload).not.toHaveProperty('python_script_id')
  })
})

describe('AgentDefForm — initial script mode (edit)', () => {
  it('shows Python Script picker when initial execution_mode is script', () => {
    renderForm({
      isCreate: false,
      initial: { execution_mode: 'script', python_script_id: 'script-1' },
    })
    expect(screen.getByRole('combobox', { name: /python script/i })).toBeInTheDocument()
    expect(screen.queryByText('Prompt Template')).not.toBeInTheDocument()
  })

  it('pre-selects the initial python_script_id', () => {
    renderForm({
      isCreate: false,
      initial: { execution_mode: 'script', python_script_id: 'script-2' },
    })
    const picker = screen.getByRole('combobox', { name: /python script/i }) as HTMLSelectElement
    expect(picker.value).toBe('script-2')
  })
})
