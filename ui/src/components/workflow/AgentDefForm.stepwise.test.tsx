import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'
import type { AgentDef } from '@/types/workflow'

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

vi.mock('@/components/workflow/PythonScriptPickerField', () => ({
  PythonScriptPickerField: ({ value, onChange }: { value: string; onChange: (v: string) => void }) => (
    <select aria-label="Python Script" value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">-- select script --</option>
      <option value="script-1">Script One</option>
    </select>
  ),
}))

// MarkdownEditor is CodeMirror-backed; a textarea stand-in keeps interaction
// simple while preserving the value/onChange contract used by the form and
// StepDefinitionEditor.
vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder?: string }) => (
    <textarea value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} />
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

async function fillPrompt(user: ReturnType<typeof userEvent.setup>, text: string) {
  await user.type(screen.getByPlaceholderText('Agent prompt template (markdown)...'), text)
}

function getPromptModeButton() {
  return screen.getByText('Prompt mode').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

function renderForm(props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}) {
  return renderWithQuery(
    <AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} {...props} />
  )
}

async function switchToStepwise(user: ReturnType<typeof userEvent.setup>) {
  await user.click(getPromptModeButton())
  await user.click(screen.getByText('Stepwise (ordered steps)'))
}

async function fillStep(user: ReturnType<typeof userEvent.setup>, stepId: string, title: string, instruction: string) {
  await user.type(screen.getByPlaceholderText('e.g., write-tests'), stepId)
  await user.type(screen.getByPlaceholderText('Short step title'), title)
  await user.type(screen.getByPlaceholderText('Step instruction (markdown)...'), instruction)
}

beforeEach(() => vi.clearAllMocks())

describe('AgentDefForm — stepwise prompt mode toggle', () => {
  it('hides the step editor by default (prompt mode full)', () => {
    renderForm()
    expect(screen.queryByText('Add step')).not.toBeInTheDocument()
  })

  it('reveals the step editor when switched to stepwise', async () => {
    const user = userEvent.setup()
    renderForm()
    await switchToStepwise(user)
    expect(screen.getByText('Add step')).toBeInTheDocument()
  })

  it('does not render the stepwise section in script mode', async () => {
    const user = userEvent.setup()
    renderForm()
    const executionModeButton = screen.getByText('Execution Mode').parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    await user.click(executionModeButton)
    await user.click(screen.getByText('Script (Python)'))
    expect(screen.queryByText('Prompt mode')).not.toBeInTheDocument()
  })
})

describe('AgentDefForm — stepwise submission', () => {
  it('emits prompt_mode: stepwise and a structured steps array', async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    renderForm({ onSubmit })

    await user.type(screen.getByPlaceholderText(/e\.g\., setup-analyzer/i), 'my-agent')
    await fillPrompt(user, 'Test prompt')
    await switchToStepwise(user)
    await user.click(screen.getByText('Add step'))
    await fillStep(user, 'write-tests', 'Write tests', 'Write the failing tests first.')

    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        prompt_mode: 'stepwise',
        steps: [
          expect.objectContaining({
            step_id: 'write-tests',
            title: 'Write tests',
            instruction: 'Write the failing tests first.',
          }),
        ],
      })
    )
  })

  it('omits steps entirely (not []) when prompt mode is full', async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    renderForm({ onSubmit })

    await user.type(screen.getByPlaceholderText(/e\.g\., setup-analyzer/i), 'my-agent')
    await fillPrompt(user, 'Test prompt')
    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ prompt_mode: 'full' }))
    const payload = onSubmit.mock.calls[0][0] as Record<string, unknown>
    expect(payload).not.toHaveProperty('steps')
  })

  it('hydrates the step editor from a JSON-string initial.steps', () => {
    renderForm({
      isCreate: false,
      initial: makeAgentDef({
        prompt_mode: 'stepwise',
        steps: '[{"step_id":"write-tests","title":"Write tests","instruction":"Do it"}]',
      }),
    })

    expect(screen.getByPlaceholderText('e.g., write-tests')).toHaveValue('write-tests')
    expect(screen.getByPlaceholderText('Short step title')).toHaveValue('Write tests')
  })

  it('blocks submit and shows inline errors for an invalid step config', async () => {
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    renderForm({ onSubmit })

    await user.type(screen.getByPlaceholderText(/e\.g\., setup-analyzer/i), 'my-agent')
    await fillPrompt(user, 'Test prompt')
    await switchToStepwise(user)
    await user.click(screen.getByText('Add step'))
    // Leave step_id empty — missing/invalid step_id blocks submission.
    await user.type(screen.getByPlaceholderText('Short step title'), 'Write tests')
    await user.type(screen.getByPlaceholderText('Step instruction (markdown)...'), 'Do it')

    expect(screen.getByText(/invalid step_id/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /create/i }))
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('renders a passed submitError verbatim', () => {
    renderForm({ submitError: 'steps: at least one step is required' })
    expect(screen.getByText('steps: at least one step is required')).toBeInTheDocument()
  })
})
