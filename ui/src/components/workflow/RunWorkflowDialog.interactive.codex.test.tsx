import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RunWorkflowDialog } from './RunWorkflowDialog'
import { renderWithQuery } from '@/test/utils'
import * as workflowApi from '@/api/workflows'
import * as agentDefsApi from '@/api/agentDefs'
import type { WorkflowDefSummary, AgentDef } from '@/types/workflow'

const mockMutateAsync = vi.fn()
const mockAddSession = vi.fn()

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

vi.mock('@/stores/interactiveSessionsStore', () => ({
  useInteractiveSessionsStore: vi.fn((selector) =>
    selector({ add: mockAddSession })
  ),
}))

const makeCodexAgentDef = (overrides: Partial<AgentDef> = {}): AgentDef => ({
  id: 'setup-analyzer',
  project_id: 'test-project',
  workflow_id: 'feature',
  model: 'codex_gpt55_high',
  timeout: 300,
  prompt: 'test',
  created_at: '',
  updated_at: '',
  ...overrides,
})

const featureWorkflow: WorkflowDefSummary = {
  description: 'Feature workflow',
  scope_type: 'ticket',
  phases: [
    { id: 'setup-analyzer', agent: 'setup-analyzer', layer: 0 },
    { id: 'implementor', agent: 'implementor', layer: 1 },
  ],
}

describe('RunWorkflowDialog — interactive/plan mode payload is CLI-agnostic', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({ feature: featureWorkflow })
    vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([makeCodexAgentDef()])
    mockMutateAsync.mockResolvedValue({ instance_id: 'i1', status: 'active', session_id: 's1' })
  })

  it('sends interactive:true then plan_mode:true for a codex L0 agent', async () => {
    const user = userEvent.setup()
    renderWithQuery(<RunWorkflowDialog open={true} onClose={vi.fn()} ticketId="TEST-1" />)

    await user.click(await screen.findByLabelText(/start interactive/i))
    await user.click(screen.getByRole('button', { name: /^run$/i }))
    await waitFor(() =>
      expect(mockMutateAsync).toHaveBeenNthCalledWith(
        1,
        expect.objectContaining({ params: expect.objectContaining({ interactive: true }) })
      )
    )

    await user.click(screen.getByLabelText(/plan before execution/i))
    await user.click(screen.getByRole('button', { name: /^run$/i }))
    await waitFor(() =>
      expect(mockMutateAsync).toHaveBeenNthCalledWith(
        2,
        expect.objectContaining({ params: expect.objectContaining({ plan_mode: true }) })
      )
    )
  })
})
