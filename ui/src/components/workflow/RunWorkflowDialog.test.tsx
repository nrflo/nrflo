import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RunWorkflowDialog } from './RunWorkflowDialog'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true }),
}))

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
}))

vi.mock('@/api/agentDefs', () => ({
  listAgentDefs: vi.fn().mockResolvedValue([]),
}))

vi.mock('@/hooks/useTickets', () => ({
  useRunWorkflow: vi.fn().mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({}),
    isPending: false,
    isError: false,
    error: null,
  }),
}))

function renderDialog(props: Partial<React.ComponentProps<typeof RunWorkflowDialog>> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <RunWorkflowDialog
        open={true}
        onClose={vi.fn()}
        ticketId="test-ticket"
        {...props}
      />
    </QueryClientProvider>
  )
}

describe('RunWorkflowDialog', () => {
  let listWorkflowDefs: ReturnType<typeof vi.fn>

  beforeEach(async () => {
    vi.clearAllMocks()
    const workflows = await import('@/api/workflows')
    listWorkflowDefs = workflows.listWorkflowDefs as ReturnType<typeof vi.fn>
    listWorkflowDefs.mockResolvedValue({
      'ticket-wf': {
        description: 'Ticket workflow',
        scope_type: 'ticket',
        phases: [{ id: 'impl', agent: 'impl', layer: 0 }],
      },
      'project-wf': {
        description: 'Project workflow',
        scope_type: 'project',
        phases: [{ id: 'setup', agent: 'setup', layer: 0 }],
      },
    })
  })

  it('shows only ticket-scoped workflows in the dropdown', async () => {
    const user = userEvent.setup()
    renderDialog()

    // Auto-selects the ticket-scoped workflow
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /ticket-wf/ })).toBeInTheDocument()
    })

    // Open the panel and verify project-wf is absent
    await user.click(screen.getByRole('button', { name: /ticket-wf/ }))

    expect(screen.queryByText(/project-wf/)).not.toBeInTheDocument()
  })

  it('treats missing scope_type as ticket (backward compat)', async () => {
    listWorkflowDefs.mockResolvedValue({
      'legacy-wf': {
        description: 'Legacy workflow',
        phases: [{ id: 'impl', agent: 'impl', layer: 0 }],
      },
      'project-only': {
        description: 'Project only',
        scope_type: 'project',
        phases: [{ id: 'setup', agent: 'setup', layer: 0 }],
      },
    })

    const user = userEvent.setup()
    renderDialog()

    // Auto-selects the no-scope (treated as ticket) workflow
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /legacy-wf/ })).toBeInTheDocument()
    })

    // Open the panel and verify project-only is absent
    await user.click(screen.getByRole('button', { name: /legacy-wf/ }))

    expect(screen.queryByText(/project-only/)).not.toBeInTheDocument()
  })

  it('shows empty state when no ticket-scoped workflows exist', async () => {
    listWorkflowDefs.mockResolvedValue({
      'project-wf': {
        description: 'Project workflow',
        scope_type: 'project',
        phases: [{ id: 'setup', agent: 'setup', layer: 0 }],
      },
    })

    renderDialog()

    await waitFor(() => {
      expect(screen.getByText(/No workflow definitions found/)).toBeInTheDocument()
    })
  })
})
