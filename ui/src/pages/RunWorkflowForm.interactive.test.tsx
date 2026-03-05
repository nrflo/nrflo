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

const featurePhases: PhaseEntry[] = [{ id: 'setup', agent: 'setup', layer: 0 }]
const featureWorkflows: [string, { description: string; scope_type?: string; phases: PhaseEntry[] }][] = [
  ['feature', { description: 'Feature workflow', scope_type: 'project', phases: featurePhases }],
]

describe('RunWorkflowForm — interactive/plan mode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  const renderForm = (
    onRun = vi.fn(),
    workflows = featureWorkflows,
    agentDefs: AgentDef[] = [makeAgentDef()],
  ) => {
    vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue(agentDefs)
    return renderWithQuery(
      <RunWorkflowForm
        projectWorkflows={workflows}
        defsLoading={false}
        selectedWorkflowDef="feature"
        onSelectWorkflowDef={vi.fn()}
        instructions=""
        onInstructionsChange={vi.fn()}
        onRun={onRun}
        runPending={false}
        runError={null}
      />
    )
  }

  describe('checkbox visibility', () => {
    it('shows Start Interactive checkbox when L0 has exactly 1 Claude agent', async () => {
      renderForm()
      expect(await screen.findByLabelText(/start interactive/i)).toBeInTheDocument()
    })

    it('shows Plan Before Execution checkbox when L0 has a Claude agent', async () => {
      renderForm()
      expect(await screen.findByLabelText(/plan before execution/i)).toBeInTheDocument()
    })

    it('hides Start Interactive checkbox when L0 agent is non-Claude', async () => {
      renderForm(vi.fn(), featureWorkflows, [makeAgentDef({ model: 'opencode_gpt_normal' })])

      await waitFor(() => expect(agentDefsApi.listAgentDefs).toHaveBeenCalledWith('feature'))
      await waitFor(() =>
        expect(screen.queryByLabelText(/start interactive/i)).not.toBeInTheDocument()
      )
    })

    it('hides Plan Before Execution checkbox when L0 agent is non-Claude', async () => {
      renderForm(vi.fn(), featureWorkflows, [makeAgentDef({ model: 'codex_gpt_high' })])

      await waitFor(() => expect(agentDefsApi.listAgentDefs).toHaveBeenCalledWith('feature'))
      await waitFor(() =>
        expect(screen.queryByLabelText(/plan before execution/i)).not.toBeInTheDocument()
      )
    })

    it('hides Start Interactive when L0 has multiple agents', async () => {
      const multiAgentWorkflows: typeof featureWorkflows = [
        [
          'feature',
          {
            description: 'Multi-agent',
            scope_type: 'project',
            phases: [
              { id: 'agent-a', agent: 'agent-a', layer: 0 },
              { id: 'agent-b', agent: 'agent-b', layer: 0 },
            ],
          },
        ],
      ]
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([
        makeAgentDef({ id: 'agent-a' }),
        makeAgentDef({ id: 'agent-b' }),
      ])
      renderWithQuery(
        <RunWorkflowForm
          projectWorkflows={multiAgentWorkflows}
          defsLoading={false}
          selectedWorkflowDef="feature"
          onSelectWorkflowDef={vi.fn()}
          instructions=""
          onInstructionsChange={vi.fn()}
          onRun={vi.fn()}
          runPending={false}
          runError={null}
        />
      )

      await waitFor(() => expect(agentDefsApi.listAgentDefs).toHaveBeenCalled())
      await waitFor(() =>
        expect(screen.queryByLabelText(/start interactive/i)).not.toBeInTheDocument()
      )
    })
  })

  describe('mutual exclusion', () => {
    it('selecting interactive unchecks plan', async () => {
      const user = userEvent.setup()
      renderForm()

      const plan = await screen.findByLabelText(/plan before execution/i)
      const interactive = await screen.findByLabelText(/start interactive/i)

      await user.click(plan)
      expect(plan).toBeChecked()

      await user.click(interactive)
      expect(interactive).toBeChecked()
      expect(plan).not.toBeChecked()
    })

    it('selecting plan unchecks interactive', async () => {
      const user = userEvent.setup()
      renderForm()

      const interactive = await screen.findByLabelText(/start interactive/i)
      const plan = await screen.findByLabelText(/plan before execution/i)

      await user.click(interactive)
      expect(interactive).toBeChecked()

      await user.click(plan)
      expect(plan).toBeChecked()
      expect(interactive).not.toBeChecked()
    })
  })

  describe('plan mode hides instructions textarea', () => {
    it('shows instructions textarea by default', async () => {
      renderForm()
      expect(await screen.findByPlaceholderText(/additional context/i)).toBeInTheDocument()
    })

    it('hides instructions textarea when plan mode selected', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.click(await screen.findByLabelText(/plan before execution/i))
      expect(screen.queryByPlaceholderText(/additional context/i)).not.toBeInTheDocument()
    })
  })

  describe('onRun called with correct startMode', () => {
    it('calls onRun with "interactive" when interactive mode selected', async () => {
      const user = userEvent.setup()
      const onRun = vi.fn()
      renderForm(onRun)

      await user.click(await screen.findByLabelText(/start interactive/i))
      await user.click(screen.getByRole('button', { name: /^run$/i }))

      expect(onRun).toHaveBeenCalledWith('interactive')
    })

    it('calls onRun with "plan" when plan mode selected', async () => {
      const user = userEvent.setup()
      const onRun = vi.fn()
      renderForm(onRun)

      await user.click(await screen.findByLabelText(/plan before execution/i))
      await user.click(screen.getByRole('button', { name: /^run$/i }))

      expect(onRun).toHaveBeenCalledWith('plan')
    })

    it('calls onRun with "normal" when no mode selected', async () => {
      const user = userEvent.setup()
      const onRun = vi.fn()
      renderForm(onRun)

      // Wait for form to be ready
      await screen.findByPlaceholderText(/additional context/i)
      await user.click(screen.getByRole('button', { name: /^run$/i }))

      expect(onRun).toHaveBeenCalledWith('normal')
    })
  })
})
