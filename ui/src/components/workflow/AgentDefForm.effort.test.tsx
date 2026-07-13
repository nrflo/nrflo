import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'
import type { AgentDef, AgentDefCreateRequest, AgentDefUpdateRequest } from '@/types/workflow'

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => false,
}))

vi.mock('@/hooks/useCLIModels', () => ({
  useModelOptions: () => [
    { label: 'Claude', options: [{ value: 'sonnet', label: 'Claude: Sonnet' }] },
  ],
  useCLIModels: () => ({
    data: [{ id: 'sonnet', cli_type: 'claude', display_name: 'Sonnet', mapped_model: 'claude-sonnet-5', reasoning_effort: 'high' }],
  }),
}))

vi.mock('@/hooks/useAPIModels', () => ({ useAPIModelOptions: () => [], useAPIModels: () => ({ data: [] }) }))

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

vi.mock('@/components/workflow/PythonScriptPickerField', () => ({
  PythonScriptPickerField: ({ value, onChange }: { value: string; onChange: (v: string) => void }) => (
    <select aria-label="Python Script" value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">-- select script --</option>
      <option value="script-1">Script One</option>
    </select>
  ),
}))

function makeAgentDef(overrides: Partial<AgentDef> = {}): AgentDef {
  return {
    id: 'test-agent',
    project_id: 'test-project',
    workflow_id: 'feature',
    layer: 0,
    model: 'sonnet',
    timeout: 20,
    prompt: 'Test prompt',
    execution_mode: 'cli_interactive',
    tools: '',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function getEffortDropdownButton() {
  const label = screen.getByText('Reasoning Effort')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

async function selectDropdownOption(
  user: ReturnType<typeof userEvent.setup>,
  triggerButton: HTMLButtonElement,
  optionLabel: string
) {
  await user.click(triggerButton)
  const dropdownContainer = triggerButton.closest('.relative')!
  const option = Array.from(dropdownContainer.querySelectorAll('.cursor-pointer span')).find(
    (el) => el.textContent === optionLabel
  ) as HTMLElement
  await user.click(option)
}

describe('AgentDefForm - reasoning effort', () => {
  it('is hidden for execution_mode=script', async () => {
    const user = userEvent.setup()
    render(<AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} />)
    await selectDropdownOption(user, screen.getByText('Execution Mode').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement, 'Script (Python)')
    expect(screen.queryByText('Reasoning Effort')).not.toBeInTheDocument()
  })

  it('puts the selected effort in the create submit payload', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<AgentDefForm isCreate={true} onSubmit={onSubmit} onCancel={vi.fn()} />)

    await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
    await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
    await selectDropdownOption(user, getEffortDropdownButton(), 'High')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ reasoning_effort: 'high' } as Partial<AgentDefCreateRequest>)
    )
  })

  it('sends reasoning_effort: null when left empty', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<AgentDefForm isCreate={true} onSubmit={onSubmit} onCancel={vi.fn()} />)

    await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
    await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ reasoning_effort: null } as Partial<AgentDefCreateRequest>)
    )
  })

  it('omits reasoning_effort entirely from the script-mode payload', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(
      <AgentDefForm
        isCreate={false}
        initial={makeAgentDef({ execution_mode: 'script', python_script_id: 'script-1' })}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />
    )

    await user.click(screen.getByRole('button', { name: /^save$/i }))

    expect(onSubmit).toHaveBeenCalled()
    const payload = onSubmit.mock.calls[0][0] as AgentDefUpdateRequest
    expect('reasoning_effort' in payload).toBe(false)
  })

  it('hydrates an existing reasoning_effort value on edit', () => {
    render(
      <AgentDefForm
        isCreate={false}
        initial={makeAgentDef({ reasoning_effort: 'max' })}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />
    )
    expect(getEffortDropdownButton().textContent).toContain('Max')
  })
})
