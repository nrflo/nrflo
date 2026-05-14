import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

const mockUseAPIModeEnabled = vi.hoisted(() => vi.fn().mockReturnValue(true))

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: mockUseAPIModeEnabled,
}))

const claudeModel = { id: 'sonnet', cli_type: 'claude', enabled: true, display_name: 'Sonnet' }
const opencodeModel = { id: 'opencode_default', cli_type: 'opencode', enabled: true, display_name: 'Opencode' }

const mockUseCLIModels = vi.hoisted(() => vi.fn())

vi.mock('@/hooks/useCLIModels', () => ({
  useModelOptions: () => [
    { value: 'sonnet', label: 'Sonnet' },
    { value: 'opencode_default', label: 'Opencode' },
  ],
  useCLIModels: () => mockUseCLIModels(),
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

vi.mock('@/components/workflow/PythonScriptPickerField', () => ({
  PythonScriptPickerField: ({ value, onChange }: { value: string; onChange: (v: string) => void }) => (
    <select aria-label="Python Script" value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">-- select script --</option>
    </select>
  ),
}))

function renderForm(props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}) {
  return render(
    <AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} {...props} />
  )
}

function getExecutionModeButton() {
  const label = screen.getByText('Execution Mode')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

function getModelButton() {
  const label = screen.getByText('Model')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

async function selectDropdownOption(
  user: ReturnType<typeof userEvent.setup>,
  trigger: HTMLButtonElement,
  label: string
) {
  await user.click(trigger)
  await user.click(screen.getByText(label))
}

function getExecutionModeOptions(trigger: HTMLButtonElement): string[] {
  const container = trigger.parentElement!.querySelector('.absolute')!
  return Array.from(container.querySelectorAll('.truncate')).map(el => el.textContent ?? '')
}

beforeEach(() => {
  mockUseAPIModeEnabled.mockReturnValue(true)
  mockUseCLIModels.mockReturnValue({ data: [claudeModel, opencodeModel] })
})

describe('AgentDefForm — opencode model execution mode filtering', () => {
  it('claude model includes cli_interactive in execution mode options', async () => {
    const user = userEvent.setup()
    renderForm({ initial: { model: 'sonnet' } })

    const trigger = getExecutionModeButton()
    await user.click(trigger)
    const opts = getExecutionModeOptions(trigger)
    expect(opts).toContain('CLI Interactive (PTY)')
  })

  it('opencode model omits cli_interactive from execution mode options', async () => {
    const user = userEvent.setup()
    renderForm({ initial: { model: 'opencode_default' } })

    const trigger = getExecutionModeButton()
    await user.click(trigger)
    const opts = getExecutionModeOptions(trigger)
    expect(opts).not.toContain('CLI Interactive (PTY)')
    expect(opts).toContain('CLI (default)')
  })

  it('opencode model retains cli, api, and script options', async () => {
    const user = userEvent.setup()
    renderForm({ initial: { model: 'opencode_default' } })

    const trigger = getExecutionModeButton()
    await user.click(trigger)
    const opts = getExecutionModeOptions(trigger)
    expect(opts).toContain('CLI (default)')
    expect(opts).toContain('API (in-process Anthropic runner)')
    expect(opts).toContain('Script (Python)')
  })

  it('switching from claude model to opencode model resets cli_interactive to cli', async () => {
    const user = userEvent.setup()
    renderForm({ initial: { model: 'sonnet' } })

    // Select cli_interactive while on claude model
    await selectDropdownOption(user, getExecutionModeButton(), 'CLI Interactive (PTY)')
    expect(getExecutionModeButton().textContent).toContain('CLI Interactive (PTY)')

    // Now switch to opencode model
    await selectDropdownOption(user, getModelButton(), 'Opencode')

    // execution mode must have been reset to cli
    expect(getExecutionModeButton().textContent).toContain('CLI (default)')
  })

  it('auto-reset to cli is reflected in submit payload', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ initial: { model: 'sonnet' }, isCreate: true, onSubmit })

    // Start with cli_interactive
    await selectDropdownOption(user, getExecutionModeButton(), 'CLI Interactive (PTY)')

    // Switch to opencode — should reset execution mode
    await selectDropdownOption(user, getModelButton(), 'Opencode')

    // Fill required fields and submit
    await user.type(screen.getByPlaceholderText(/e\.g\., setup-analyzer/i), 'my-agent')
    await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'My prompt')
    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        execution_mode: 'cli',
        model: 'opencode_default',
      })
    )
  })

  it('initializing with opencode model and cli_interactive resets to cli on mount', () => {
    renderForm({ initial: { model: 'opencode_default', execution_mode: 'cli_interactive' } })
    expect(getExecutionModeButton().textContent).toContain('CLI (default)')
  })

  it('switching from opencode back to claude model restores cli_interactive option', async () => {
    const user = userEvent.setup()
    renderForm({ initial: { model: 'opencode_default' } })

    // Confirm cli_interactive absent
    await user.click(getExecutionModeButton())
    expect(getExecutionModeOptions(getExecutionModeButton())).not.toContain('CLI Interactive (PTY)')
    // Close dropdown
    await user.keyboard('{Escape}')

    // Switch to claude model
    await selectDropdownOption(user, getModelButton(), 'Sonnet')

    // cli_interactive should now be available
    await user.click(getExecutionModeButton())
    expect(getExecutionModeOptions(getExecutionModeButton())).toContain('CLI Interactive (PTY)')
  })
})
