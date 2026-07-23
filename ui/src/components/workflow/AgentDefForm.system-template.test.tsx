import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'
import type { AgentDef, AgentDefCreateRequest, AgentDefUpdateRequest } from '@/types/workflow'

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => false,
}))

vi.mock('@/hooks/useModels', () => ({
  useModelOptions: () => [
    { label: 'Anthropic', options: [{ value: 'sonnet-5', label: 'Anthropic: Sonnet' }] },
  ],
  useModels: () => ({
    data: [{ id: 'sonnet-5', provider: 'anthropic', display_name: 'Sonnet', cli_model: 'claude-sonnet-5', cli_efforts: [], api_efforts: [], default_effort: '' }],
  }),
}))

vi.mock('@/hooks/useDefaultTemplates', () => ({
  useInjectableTemplates: () => ({
    data: [{ id: 'tier-t0-decider', name: 'Tier T0 Decider', type: 'injectable', template: '', readonly: true, created_at: '', updated_at: '' }],
  }),
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
    model: 'sonnet-5',
    timeout: 20,
    prompt: 'Test prompt',
    execution_mode: 'cli_interactive',
    tools: '',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function getSystemTemplateDropdownButton() {
  const label = screen.getByText('System template')
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

describe('AgentDefForm - system template', () => {
  it('is hidden for execution_mode=script', async () => {
    const user = userEvent.setup()
    renderWithQuery(<AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} />)
    await selectDropdownOption(user, screen.getByText('Execution Mode').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement, 'Script (Python)')
    expect(screen.queryByText('System template')).not.toBeInTheDocument()
  })

  it('omits system_template_id from the create payload when left at default', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderWithQuery(<AgentDefForm isCreate={true} onSubmit={onSubmit} onCancel={vi.fn()} />)

    await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
    await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    expect(onSubmit).toHaveBeenCalled()
    const payload = onSubmit.mock.calls[0][0] as AgentDefCreateRequest
    expect(JSON.parse(JSON.stringify(payload))).not.toHaveProperty('system_template_id')
  })

  it('puts the selected template id in the create submit payload', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderWithQuery(<AgentDefForm isCreate={true} onSubmit={onSubmit} onCancel={vi.fn()} />)

    await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
    await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
    await selectDropdownOption(user, getSystemTemplateDropdownButton(), 'Tier T0 Decider')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ system_template_id: 'tier-t0-decider' } as Partial<AgentDefCreateRequest>)
    )
  })

  it('omits system_template_id entirely from the script-mode payload', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderWithQuery(
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
    expect('system_template_id' in payload).toBe(false)
  })


  it('hydrates an existing system_template_id value on edit', () => {
    renderWithQuery(
      <AgentDefForm
        isCreate={false}
        initial={makeAgentDef({ system_template_id: 'tier-t0-decider' })}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />
    )
    expect(getSystemTemplateDropdownButton().textContent).toContain('Tier T0 Decider')
  })
})
