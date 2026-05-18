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
    { label: 'Claude', options: [
      { value: 'sonnet', label: 'Claude: Sonnet' },
    ]},
  ],
  useCLIModels: () => ({ data: [] }),
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

describe('AgentDefForm - validation_commands', () => {
  describe('initial rendering', () => {
    it('renders Add command button with no rows when no initial commands', () => {
      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      expect(screen.getByText('Add command')).toBeInTheDocument()
      expect(screen.queryByPlaceholderText('e.g., make test')).not.toBeInTheDocument()
    })

    it('seeds rows from JSON string in initial.validation_commands', () => {
      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ validation_commands: '["make test","make lint"]' })}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      const inputs = screen.getAllByPlaceholderText('e.g., make test')
      expect(inputs).toHaveLength(2)
      expect(inputs[0]).toHaveValue('make test')
      expect(inputs[1]).toHaveValue('make lint')
    })

    it('falls back to empty list on invalid JSON', () => {
      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ validation_commands: 'not-json' })}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      expect(screen.queryByPlaceholderText('e.g., make test')).not.toBeInTheDocument()
      expect(screen.getByText('Add command')).toBeInTheDocument()
    })
  })

  describe('adding and removing rows', () => {
    it('adds an empty row on Add command click', async () => {
      const user = userEvent.setup()
      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      await user.click(screen.getByText('Add command'))

      expect(screen.getByPlaceholderText('e.g., make test')).toBeInTheDocument()
      expect(screen.getByText('Remove')).toBeInTheDocument()
    })

    it('appends another row on second Add command click', async () => {
      const user = userEvent.setup()
      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      await user.click(screen.getByText('Add command'))
      await user.click(screen.getByText('Add command'))

      expect(screen.getAllByPlaceholderText('e.g., make test')).toHaveLength(2)
    })

    it('typing in a row updates its value', async () => {
      const user = userEvent.setup()
      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      await user.click(screen.getByText('Add command'))
      await user.type(screen.getByPlaceholderText('e.g., make test'), 'make test')

      expect(screen.getByPlaceholderText('e.g., make test')).toHaveValue('make test')
    })

    it('Remove button deletes that row', async () => {
      const user = userEvent.setup()
      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ validation_commands: '["make test"]' })}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      expect(screen.getByPlaceholderText('e.g., make test')).toBeInTheDocument()
      await user.click(screen.getByText('Remove'))
      expect(screen.queryByPlaceholderText('e.g., make test')).not.toBeInTheDocument()
    })

    it('Add command is disabled when 20 rows exist', () => {
      const cmds = JSON.stringify(Array.from({ length: 20 }, (_, i) => `cmd-${i}`))
      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ validation_commands: cmds })}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      expect(screen.getByText('Add command')).toBeDisabled()
    })
  })

  describe('submit payload', () => {
    it('sends empty array when no commands added (create)', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText('e.g., setup-analyzer'), 'new-agent')
      await user.click(screen.getByText('Create'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ validation_commands: [] } as Partial<AgentDefCreateRequest>)
      )
    })

    it('sends trimmed, non-empty commands in create payload', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText('e.g., setup-analyzer'), 'new-agent')
      await user.click(screen.getByText('Add command'))
      await user.type(screen.getByPlaceholderText('e.g., make test'), '  make test  ')
      await user.click(screen.getByText('Create'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ validation_commands: ['make test'] } as Partial<AgentDefCreateRequest>)
      )
    })

    it('drops empty rows from submit payload', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      render(
        <AgentDefForm
          isCreate={true}
          initial={{ prompt: 'test' }}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.type(screen.getByPlaceholderText('e.g., setup-analyzer'), 'new-agent')
      await user.click(screen.getByText('Add command'))
      // Leave the added row blank
      await user.click(screen.getByText('Create'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ validation_commands: [] } as Partial<AgentDefCreateRequest>)
      )
    })

    it('sends commands in update payload', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ validation_commands: '["make test","make lint"]' })}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.click(screen.getByText('Save'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ validation_commands: ['make test', 'make lint'] } as Partial<AgentDefUpdateRequest>)
      )
    })

    it('sends commands in script execution mode payload', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      render(
        <AgentDefForm
          isCreate={false}
          initial={makeAgentDef({ validation_commands: '["make test"]', execution_mode: 'script', python_script_id: 'script-1' })}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
        />
      )

      await user.click(screen.getByText('Save'))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ validation_commands: ['make test'] } as Partial<AgentDefUpdateRequest>)
      )
    })
  })
})
