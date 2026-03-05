import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RunWorkflowDialog } from './RunWorkflowDialog'
import { renderWithQuery } from '@/test/utils'
import * as workflowApi from '@/api/workflows'
import * as agentDefsApi from '@/api/agentDefs'
import type { WorkflowDefSummary, AgentDef } from '@/types/workflow'

const mockMutateAsync = vi.fn()

vi.mock('@/hooks/useTickets', () => ({
  useRunWorkflow: () => ({
    mutateAsync: mockMutateAsync,
    isPending: false,
    isError: false,
    error: null,
  }),
}))

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
}))

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

const makeAgentDef = (overrides: Partial<AgentDef> = {}): AgentDef => ({
  id: 'setup-analyzer',
  project_id: 'test-project',
  workflow_id: 'feature',
  model: 'sonnet',
  timeout: 300,
  prompt: 'test',
  created_at: '',
  updated_at: '',
  ...overrides,
})

const featureWorkflow: WorkflowDefSummary = {
  description: 'Feature workflow',
  scope_type: 'ticket',
  phases: [{ id: 'setup-analyzer', agent: 'setup-analyzer', layer: 0 }],
}

describe('RunWorkflowDialog — interactive/plan mode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({ feature: featureWorkflow })
  })

  const renderDialog = (props: Partial<React.ComponentProps<typeof RunWorkflowDialog>> = {}) =>
    renderWithQuery(
      <RunWorkflowDialog open={true} onClose={vi.fn()} ticketId="TEST-1" {...props} />
    )

  describe('checkbox visibility', () => {
    it('shows Start Interactive checkbox when L0 has exactly 1 Claude agent', async () => {
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeAgentDef()])
      renderDialog()

      expect(await screen.findByLabelText(/start interactive/i)).toBeInTheDocument()
    })

    it('shows Plan Before Execution checkbox when L0 has a Claude agent', async () => {
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeAgentDef()])
      renderDialog()

      expect(await screen.findByLabelText(/plan before execution/i)).toBeInTheDocument()
    })

    it('hides Start Interactive checkbox when L0 agent is non-Claude', async () => {
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([
        makeAgentDef({ model: 'opencode_gpt_normal' }),
      ])
      renderDialog()

      // Wait for agents to load — no Claude model means no checkboxes
      await waitFor(() => expect(agentDefsApi.listAgentDefs).toHaveBeenCalledWith('feature'))
      await waitFor(() =>
        expect(screen.queryByLabelText(/start interactive/i)).not.toBeInTheDocument()
      )
    })

    it('hides Plan Before Execution checkbox when L0 agent is non-Claude', async () => {
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([
        makeAgentDef({ model: 'codex_gpt_high' }),
      ])
      renderDialog()

      await waitFor(() => expect(agentDefsApi.listAgentDefs).toHaveBeenCalledWith('feature'))
      await waitFor(() =>
        expect(screen.queryByLabelText(/plan before execution/i)).not.toBeInTheDocument()
      )
    })

    it('hides Start Interactive when L0 has multiple agents (fan-out)', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({
        'multi-agent': {
          description: 'Multi-agent workflow',
          scope_type: 'ticket',
          phases: [
            { id: 'agent-a', agent: 'agent-a', layer: 0 },
            { id: 'agent-b', agent: 'agent-b', layer: 0 },
          ],
        },
      })
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([
        makeAgentDef({ id: 'agent-a', workflow_id: 'multi-agent' }),
        makeAgentDef({ id: 'agent-b', workflow_id: 'multi-agent' }),
      ])
      renderDialog()

      // Plan checkbox may appear (Claude L0 agents exist), but interactive should not
      await waitFor(() => expect(agentDefsApi.listAgentDefs).toHaveBeenCalled())
      await waitFor(() =>
        expect(screen.queryByLabelText(/start interactive/i)).not.toBeInTheDocument()
      )
    })
  })

  describe('mutual exclusion', () => {
    it('selecting interactive unchecks plan', async () => {
      const user = userEvent.setup()
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeAgentDef()])
      renderDialog()

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
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeAgentDef()])
      renderDialog()

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
    it('shows instructions by default', async () => {
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeAgentDef()])
      renderDialog()

      expect(await screen.findByPlaceholderText(/additional context/i)).toBeInTheDocument()
    })

    it('hides instructions when plan mode is selected', async () => {
      const user = userEvent.setup()
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeAgentDef()])
      renderDialog()

      await user.click(await screen.findByLabelText(/plan before execution/i))
      expect(screen.queryByPlaceholderText(/additional context/i)).not.toBeInTheDocument()
    })
  })

  describe('form submission', () => {
    it('sends interactive:true when interactive mode selected', async () => {
      const user = userEvent.setup()
      mockMutateAsync.mockResolvedValue({ instance_id: 'i1', status: 'active', session_id: 's1' })
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeAgentDef()])
      renderDialog()

      await user.click(await screen.findByLabelText(/start interactive/i))
      await user.click(screen.getByRole('button', { name: /^run$/i }))

      await waitFor(() =>
        expect(mockMutateAsync).toHaveBeenCalledWith(
          expect.objectContaining({
            params: expect.objectContaining({ interactive: true }),
          })
        )
      )
    })

    it('sends plan_mode:true when plan mode selected', async () => {
      const user = userEvent.setup()
      mockMutateAsync.mockResolvedValue({ instance_id: 'i1', status: 'active', session_id: 's1' })
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeAgentDef()])
      renderDialog()

      await user.click(await screen.findByLabelText(/plan before execution/i))
      await user.click(screen.getByRole('button', { name: /^run$/i }))

      await waitFor(() =>
        expect(mockMutateAsync).toHaveBeenCalledWith(
          expect.objectContaining({
            params: expect.objectContaining({ plan_mode: true }),
          })
        )
      )
    })

    it('calls onInteractiveStart with session_id and l0 agent type for interactive mode', async () => {
      const user = userEvent.setup()
      const onInteractiveStart = vi.fn()
      mockMutateAsync.mockResolvedValue({ instance_id: 'i1', status: 'active', session_id: 'sess-123' })
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeAgentDef()])
      renderDialog({ onInteractiveStart })

      await user.click(await screen.findByLabelText(/start interactive/i))
      await user.click(screen.getByRole('button', { name: /^run$/i }))

      await waitFor(() =>
        expect(onInteractiveStart).toHaveBeenCalledWith('sess-123', 'setup-analyzer')
      )
    })

    it('calls onInteractiveStart with "planner" agent type for plan mode', async () => {
      const user = userEvent.setup()
      const onInteractiveStart = vi.fn()
      mockMutateAsync.mockResolvedValue({ instance_id: 'i1', status: 'active', session_id: 'sess-456' })
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeAgentDef()])
      renderDialog({ onInteractiveStart })

      await user.click(await screen.findByLabelText(/plan before execution/i))
      await user.click(screen.getByRole('button', { name: /^run$/i }))

      await waitFor(() =>
        expect(onInteractiveStart).toHaveBeenCalledWith('sess-456', 'planner')
      )
    })

    it('does not call onInteractiveStart in normal mode', async () => {
      const user = userEvent.setup()
      const onInteractiveStart = vi.fn()
      mockMutateAsync.mockResolvedValue({ instance_id: 'i1', status: 'active' })
      vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeAgentDef()])
      renderDialog({ onInteractiveStart })

      // Wait for workflows to load (Run button becomes enabled)
      await waitFor(() =>
        expect(screen.getByRole('button', { name: /^run$/i })).not.toBeDisabled()
      )
      await user.click(screen.getByRole('button', { name: /^run$/i }))

      await waitFor(() => expect(mockMutateAsync).toHaveBeenCalled())
      expect(onInteractiveStart).not.toHaveBeenCalled()
    })
  })
})
