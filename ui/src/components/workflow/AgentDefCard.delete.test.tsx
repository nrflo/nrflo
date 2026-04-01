import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentDefCard } from './AgentDefCard'
import * as agentDefsApi from '@/api/agentDefs'
import type { AgentDef } from '@/types/workflow'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn(() => 'test-project'),
}))

vi.mock('@/api/agentDefs', () => ({
  updateAgentDef: vi.fn(),
  deleteAgentDef: vi.fn(),
}))

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value }: { value: string }) => <div>{value}</div>,
}))

vi.mock('@/components/workflow/AgentDefForm', () => ({
  AgentDefForm: () => <div data-testid="agent-def-form">AgentDefForm</div>,
}))

function makeAgentDef(overrides: Partial<AgentDef> = {}): AgentDef {
  return {
    id: 'my-agent',
    project_id: 'test-project',
    workflow_id: 'feature',
    model: 'sonnet',
    timeout: 20,
    prompt: 'Test prompt',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderCard(def: AgentDef = makeAgentDef()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentDefCard def={def} workflowId="feature" groups={[]} />
    </QueryClientProvider>
  )
}

describe('AgentDefCard - delete confirmation dialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('clicking delete button opens ConfirmDialog with agent id in message', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(screen.getByTitle('Delete'))

    expect(await screen.findByText('Delete Agent Definition')).toBeInTheDocument()
    expect(screen.getByText("Delete agent definition 'my-agent'?")).toBeInTheDocument()
  })

  it('clicking Cancel closes dialog without deleting', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(screen.getByTitle('Delete'))
    await screen.findByText('Delete Agent Definition')

    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() => expect(screen.queryByText('Delete Agent Definition')).not.toBeInTheDocument())
    expect(agentDefsApi.deleteAgentDef).not.toHaveBeenCalled()
  })

  it('clicking Delete in dialog calls deleteAgentDef', async () => {
    const user = userEvent.setup()
    vi.mocked(agentDefsApi.deleteAgentDef).mockResolvedValue(undefined)
    renderCard()

    await user.click(screen.getByTitle('Delete'))
    await screen.findByText('Delete Agent Definition')

    // Dialog portals to document.body; scope to the dialog overlay to avoid
    // ambiguity with the icon button that also has accessible name 'Delete'
    const overlay = document.querySelector<HTMLElement>('.fixed.inset-0')!
    await user.click(within(overlay).getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(agentDefsApi.deleteAgentDef).toHaveBeenCalledWith('feature', 'my-agent')
    })
  })
})
