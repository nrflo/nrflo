import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { RunWorkflowDialog } from './RunWorkflowDialog'
import { renderWithQuery } from '@/test/utils'
import * as workflowApi from '@/api/workflows'
import * as agentDefsApi from '@/api/agentDefs'
import type { WorkflowDefSummary } from '@/types/workflow'

vi.mock('@/hooks/useTickets', () => ({
  useRunWorkflow: () => ({
    mutateAsync: vi.fn(),
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

const featureWorkflow: WorkflowDefSummary = {
  description: 'Feature workflow',
  scope_type: 'ticket',
  phases: [{ id: 'implementor', agent: 'implementor', layer: 0 }],
}

describe('RunWorkflowDialog — blockedReason prop', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({ feature: featureWorkflow })
    vi.mocked(agentDefsApi.listAgentDefs).mockResolvedValue([])
  })

  const renderDialog = (props: Partial<React.ComponentProps<typeof RunWorkflowDialog>> = {}) =>
    renderWithQuery(
      <RunWorkflowDialog open={true} onClose={vi.fn()} ticketId="TEST-1" {...props} />
    )

  describe('Run button disabled state', () => {
    it('disables Run button when blockedReason is set', () => {
      renderDialog({ blockedReason: 'cannot run workflow on closed ticket' })

      expect(screen.getByRole('button', { name: /^run$/i })).toBeDisabled()
    })

    it('disables Run button for blocked ticket reason', () => {
      renderDialog({ blockedReason: 'cannot run workflow on blocked ticket — blocked by: TICKET-2' })

      expect(screen.getByRole('button', { name: /^run$/i })).toBeDisabled()
    })

    it('Run button becomes enabled after workflow loads when no blockedReason', async () => {
      renderDialog()

      // Initially disabled (no workflow selected)
      expect(screen.getByRole('button', { name: /^run$/i })).toBeDisabled()

      // After workflow loads, button should be enabled
      await waitFor(() =>
        expect(screen.getByRole('button', { name: /^run$/i })).not.toBeDisabled()
      )
    })

    it('Run button stays disabled even after workflow loads when blockedReason is set', async () => {
      renderDialog({ blockedReason: 'cannot run workflow on closed ticket' })

      await waitFor(() => expect(workflowApi.listWorkflowDefs).toHaveBeenCalled())

      expect(screen.getByRole('button', { name: /^run$/i })).toBeDisabled()
    })
  })

  describe('yellow warning banner', () => {
    it('shows yellow warning banner with blockedReason text', () => {
      const reason = 'cannot run workflow on closed ticket'
      renderDialog({ blockedReason: reason })

      expect(screen.getByText(reason)).toBeInTheDocument()
    })

    it('shows blocked ticket message in banner', () => {
      const reason = 'cannot run workflow on blocked ticket — blocked by: DEP-1, DEP-2'
      renderDialog({ blockedReason: reason })

      expect(screen.getByText(reason)).toBeInTheDocument()
    })

    it('does not show yellow banner when blockedReason is undefined', async () => {
      renderDialog()

      await waitFor(() => expect(workflowApi.listWorkflowDefs).toHaveBeenCalled())

      expect(screen.queryByText(/cannot run workflow/i)).not.toBeInTheDocument()
    })

    it('does not show yellow banner when blockedReason is empty string', async () => {
      renderDialog({ blockedReason: '' })

      await waitFor(() => expect(workflowApi.listWorkflowDefs).toHaveBeenCalled())

      // Empty string is falsy — no banner rendered
      expect(screen.queryByText(/cannot run workflow/i)).not.toBeInTheDocument()
    })
  })
})
