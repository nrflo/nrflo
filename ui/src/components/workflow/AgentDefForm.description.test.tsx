import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'
import type { AgentDef, AgentDefCreateRequest, AgentDefUpdateRequest } from '@/types/workflow'

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => false,
}))

vi.mock('@/hooks/useDefaultTemplates', () => ({
  useInjectableTemplates: () => ({ data: [] }),
}))

vi.mock('@/hooks/useModels', () => ({
  useModelOptions: () => [
    { label: 'Anthropic', options: [{ value: 'sonnet-5', label: 'Anthropic: Sonnet' }] },
  ],
  useModels: () => ({ data: [] }),
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

function getNodeRoleDropdownButton() {
  const label = screen.getByText('Node role')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

function getDescriptionInput() {
  return screen.getByPlaceholderText('When to use this agent (tool-description quality)')
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

describe('AgentDefForm - node role and description', () => {
  describe('rendering', () => {
    it('node role dropdown defaults to Static', () => {
      renderWithQuery(<AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} />)
      expect(getNodeRoleDropdownButton().textContent).toContain('Static (runs as a workflow phase)')
    })

    it('description input renders empty by default', () => {
      renderWithQuery(<AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} />)
      expect(getDescriptionInput()).toHaveValue('')
    })

    it('pre-selects initial node_role and description', () => {
      renderWithQuery(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ node_role: 'fanout_template', description: 'Handles fanout work' })}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )
      expect(getNodeRoleDropdownButton().textContent).toContain('Fanout template')
      expect(getDescriptionInput()).toHaveValue('Handles fanout work')
    })

    it('description field is not marked required for the default static role', () => {
      renderWithQuery(<AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} />)
      expect(screen.queryByText(/required for fanout templates/i)).not.toBeInTheDocument()
    })

    it('description field shows the required hint when node_role is fanout_template', async () => {
      const user = userEvent.setup()
      renderWithQuery(<AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} />)

      await selectDropdownOption(user, getNodeRoleDropdownButton(), 'Fanout template (bindable by plan nodes)')

      expect(screen.getByText(/required for fanout templates/i)).toBeInTheDocument()
    })
  })

  describe('submit payload', () => {
    it('omits node_role and description when left at defaults (create)', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderWithQuery(<AgentDefForm isCreate={true} onSubmit={onSubmit} onCancel={vi.fn()} />)

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ node_role: undefined, description: undefined } as Partial<AgentDefCreateRequest>)
      )
    })

    it('sends node_role and description in the create payload', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderWithQuery(<AgentDefForm isCreate={true} onSubmit={onSubmit} onCancel={vi.fn()} />)

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
      await selectDropdownOption(user, getNodeRoleDropdownButton(), 'Fanout template (bindable by plan nodes)')
      await user.type(getDescriptionInput(), 'Handles fanout work')
      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          node_role: 'fanout_template',
          description: 'Handles fanout work',
        } as Partial<AgentDefCreateRequest>)
      )
    })

    it('sends node_role and description in the update payload', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderWithQuery(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef()}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await selectDropdownOption(user, getNodeRoleDropdownButton(), 'Planner (drafts plan manifests)')
      await user.type(getDescriptionInput(), 'Drafts the plan')
      await user.click(screen.getByRole('button', { name: /^save$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          node_role: 'planner',
          description: 'Drafts the plan',
        } as Partial<AgentDefUpdateRequest>)
      )
    })

    it('trims whitespace-only description to undefined', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderWithQuery(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef()}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(getDescriptionInput(), '   ')
      await user.click(screen.getByRole('button', { name: /^save$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ description: undefined } as Partial<AgentDefUpdateRequest>)
      )
    })

    it('sends description in the script execution mode payload', async () => {
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

      await selectDropdownOption(user, getNodeRoleDropdownButton(), 'Fanout template (bindable by plan nodes)')
      await user.type(getDescriptionInput(), 'Runs a script fanout')
      await user.click(screen.getByRole('button', { name: /^save$/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          node_role: 'fanout_template',
          description: 'Runs a script fanout',
        } as Partial<AgentDefUpdateRequest>)
      )
    })
  })

  describe('submit blocking for fanout_template without a description', () => {
    it('blocks submit when node_role is fanout_template and description is blank', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderWithQuery(<AgentDefForm isCreate={true} onSubmit={onSubmit} onCancel={vi.fn()} />)

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
      await selectDropdownOption(user, getNodeRoleDropdownButton(), 'Fanout template (bindable by plan nodes)')
      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).not.toHaveBeenCalled()
    })

    it('blocks submit when description is only whitespace', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderWithQuery(<AgentDefForm isCreate={true} onSubmit={onSubmit} onCancel={vi.fn()} />)

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
      await selectDropdownOption(user, getNodeRoleDropdownButton(), 'Fanout template (bindable by plan nodes)')
      await user.type(getDescriptionInput(), '   ')
      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).not.toHaveBeenCalled()
    })

    it('allows submit once a non-blank description is provided', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderWithQuery(<AgentDefForm isCreate={true} onSubmit={onSubmit} onCancel={vi.fn()} />)

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
      await selectDropdownOption(user, getNodeRoleDropdownButton(), 'Fanout template (bindable by plan nodes)')
      await user.type(getDescriptionInput(), 'Selectable by the planner')
      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).toHaveBeenCalled()
    })

    it('does not block submit for planner role with a blank description', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderWithQuery(<AgentDefForm isCreate={true} onSubmit={onSubmit} onCancel={vi.fn()} />)

      await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByLabelText('Prompt Template'), 'Test prompt')
      await selectDropdownOption(user, getNodeRoleDropdownButton(), 'Planner (drafts plan manifests)')
      await user.click(screen.getByRole('button', { name: /^create$/i }))

      expect(onSubmit).toHaveBeenCalled()
    })
  })
})
