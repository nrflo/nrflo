import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RunWorkflowForm } from './ProjectWorkflowComponents'
import { renderWithQuery } from '@/test/utils'
import * as agentDefsApi from '@/api/agentDefs'
import type { AgentDef, WorkflowDefSummary } from '@/types/workflow'

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn(),
}))

const makeAgentDef = (overrides: Partial<AgentDef> = {}): AgentDef => ({
  id: 'setup',
  project_id: 'test-project',
  workflow_id: 'feature',
  model: 'sonnet',
  timeout: 300,
  prompt: 'test',
  created_at: '',
  updated_at: '',
  ...overrides,
})

type PhaseEntry = WorkflowDefSummary['phases'][number]

const projectPhases: PhaseEntry[] = [
  { id: 'setup', agent: 'setup', layer: 0 },
  { id: 'impl', agent: 'impl', layer: 1 },
]

const projectScopedWorkflows: [string, { description: string; scope_type?: string; phases: PhaseEntry[] }][] = [
  ['feature', { description: 'Project feature', scope_type: 'project', phases: projectPhases }],
]

const ticketScopedWorkflows: [string, { description: string; scope_type?: string; phases: PhaseEntry[] }][] = [
  ['feature', { description: 'Ticket feature', scope_type: 'ticket', phases: projectPhases }],
]

function renderForm({
  onRun = vi.fn(),
  onInstructionsChange = vi.fn(),
  instructions = '',
  workflows = projectScopedWorkflows,
  agentDefs = [makeAgentDef(), makeAgentDef({ id: 'impl' })],
}: {
  onRun?: (mode: 'normal' | 'interactive' | 'plan' | 'endless') => void
  onInstructionsChange?: (v: string) => void
  instructions?: string
  workflows?: typeof projectScopedWorkflows
  agentDefs?: AgentDef[]
} = {}) {
  vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue(agentDefs)
  return renderWithQuery(
    <RunWorkflowForm
      projectWorkflows={workflows}
      defsLoading={false}
      selectedWorkflowDef="feature"
      onSelectWorkflowDef={vi.fn()}
      instructions={instructions}
      onInstructionsChange={onInstructionsChange}
      onRun={onRun}
      runPending={false}
      runError={null}
    />
  )
}

describe('RunWorkflowForm — endless loop mode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows Endless loop checkbox only for project-scoped workflows', async () => {
    renderForm()
    expect(await screen.findByLabelText(/endless loop/i)).toBeInTheDocument()
  })

  it('does not show Endless loop checkbox for ticket-scoped workflows', async () => {
    renderForm({ workflows: ticketScopedWorkflows })
    // Wait for agent defs to resolve so the mode checkboxes have had a chance to render.
    await waitFor(() => expect(agentDefsApi.listAgentDefs).toHaveBeenCalledWith('feature'))
    expect(screen.queryByLabelText(/endless loop/i)).not.toBeInTheDocument()
  })

  it('clicking Endless loop hides and clears the Instructions textarea', async () => {
    const user = userEvent.setup()
    const onInstructionsChange = vi.fn()
    renderForm({ onInstructionsChange, instructions: 'prior text' })

    // Textarea shown initially.
    expect(await screen.findByPlaceholderText(/additional context/i)).toBeInTheDocument()

    await user.click(await screen.findByLabelText(/endless loop/i))

    // Textarea hidden after toggling endless loop.
    expect(screen.queryByPlaceholderText(/additional context/i)).not.toBeInTheDocument()
    // Parent gets cleared instructions.
    expect(onInstructionsChange).toHaveBeenCalledWith('')
  })

  it('unchecking Endless loop restores the Instructions textarea', async () => {
    const user = userEvent.setup()
    renderForm()

    const endless = await screen.findByLabelText(/endless loop/i)
    await user.click(endless)
    expect(screen.queryByPlaceholderText(/additional context/i)).not.toBeInTheDocument()

    await user.click(endless)
    expect(await screen.findByPlaceholderText(/additional context/i)).toBeInTheDocument()
  })

  it('disables Interactive and Plan checkboxes when Endless loop is checked', async () => {
    const user = userEvent.setup()
    renderForm()

    const interactive = await screen.findByLabelText(/start interactive/i)
    const plan = await screen.findByLabelText(/plan before execution/i)
    const endless = await screen.findByLabelText(/endless loop/i)

    // Baseline: both enabled.
    expect(interactive).not.toBeDisabled()
    expect(plan).not.toBeDisabled()

    await user.click(endless)

    expect(interactive).toBeDisabled()
    expect(plan).toBeDisabled()

    // Re-enable by unchecking endless loop.
    await user.click(endless)
    expect(interactive).not.toBeDisabled()
    expect(plan).not.toBeDisabled()
  })

  it('Run button emits startMode="endless" after selecting endless loop', async () => {
    const user = userEvent.setup()
    const onRun = vi.fn()
    renderForm({ onRun })

    await user.click(await screen.findByLabelText(/endless loop/i))
    await user.click(screen.getByRole('button', { name: /^run$/i }))

    expect(onRun).toHaveBeenCalledWith('endless')
  })

  it('Endless loop stays independent of Interactive/Plan mutual exclusion', async () => {
    const user = userEvent.setup()
    renderForm()

    const interactive = await screen.findByLabelText(/start interactive/i)
    const plan = await screen.findByLabelText(/plan before execution/i)
    const endless = await screen.findByLabelText(/endless loop/i)

    // Enabling interactive then plan still leaves endless unchecked
    // (mutual exclusion between interactive/plan is unchanged by endless being available).
    await user.click(interactive)
    await user.click(plan)
    expect(endless).not.toBeChecked()
    expect(plan).toBeChecked()
    expect(interactive).not.toBeChecked()
  })
})
