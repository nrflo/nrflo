import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RunningAgentsIndicator } from './RunningAgentsIndicator'
import type { RunningAgent, RunningAgentsResponse } from '@/types/agents'

const mockNavigate = vi.fn()
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return { ...actual, useNavigate: () => mockNavigate }
})

const mockSetCurrentProject = vi.fn()
let mockCurrentProject = 'proj-1'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: Object.assign(
    (selector: (s: { currentProject: string }) => unknown) =>
      selector({ currentProject: mockCurrentProject }),
    { getState: () => ({ setCurrentProject: mockSetCurrentProject }) },
  ),
}))

let mockRunningAgents: { data: RunningAgentsResponse | undefined } = { data: undefined }
vi.mock('@/hooks/useRunningAgents', () => ({
  useRunningAgents: () => mockRunningAgents,
}))

function makeAgent(overrides: Partial<RunningAgent> = {}): RunningAgent {
  return {
    session_id: 'sess-1',
    project_id: 'proj-1',
    project_name: 'Alpha Project',
    ticket_id: 'ticket-1',
    workflow_id: 'feature',
    agent_type: 'implementor',
    model_id: 'sonnet',
    phase: 'implement',
    started_at: '2026-01-01T00:00:00Z',
    elapsed_sec: 150,
    ...overrides,
  }
}

function renderIndicator() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <RunningAgentsIndicator />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function getTrigger() {
  return screen.getByRole('status').parentElement!
}

describe('RunningAgentsIndicator', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockCurrentProject = 'proj-1'
    mockRunningAgents = { data: undefined }
  })

  it('renders nothing when data is undefined', () => {
    const { container } = renderIndicator()
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when count is 0', () => {
    mockRunningAgents = { data: { agents: [], count: 0 } }
    const { container } = renderIndicator()
    expect(container.firstChild).toBeNull()
  })

  it('shows spinner and count badge when agents running', () => {
    mockRunningAgents = { data: { agents: [makeAgent()], count: 1 } }
    renderIndicator()
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.getByText('1')).toBeInTheDocument()
  })

  it('shows correct count for multiple agents', () => {
    mockRunningAgents = {
      data: { agents: [makeAgent(), makeAgent({ session_id: 's2' })], count: 2 },
    }
    renderIndicator()
    expect(screen.getByText('2')).toBeInTheDocument()
  })

  describe('popover', () => {
    beforeEach(() => {
      mockRunningAgents = {
        data: {
          agents: [
            makeAgent({
              session_id: 'sess-1',
              project_name: 'Alpha Project',
              workflow_id: 'feature',
              agent_type: 'implementor',
              elapsed_sec: 150,
              ticket_id: 'ticket-1',
            }),
            makeAgent({
              session_id: 'sess-2',
              project_id: 'proj-2',
              project_name: 'Beta Project',
              workflow_id: 'bugfix',
              agent_type: 'qa-verifier',
              elapsed_sec: 3661,
              ticket_id: '',
            }),
          ],
          count: 2,
        },
      }
    })

    it('does not show popover before hover', () => {
      renderIndicator()
      expect(screen.queryByText('Running Agents (2)')).not.toBeInTheDocument()
    })

    it('shows popover with title and grouped projects on mouseenter', () => {
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText('Running Agents (2)')).toBeInTheDocument()
      expect(screen.getByText('Alpha Project')).toBeInTheDocument()
      expect(screen.getByText('Beta Project')).toBeInTheDocument()
    })

    it('shows workflow/agent_type and formatted elapsed in popover', () => {
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText(/feature \/ implementor/)).toBeInTheDocument()
      expect(screen.getByText('(2m 30s)')).toBeInTheDocument()
      expect(screen.getByText(/bugfix \/ qa-verifier/)).toBeInTheDocument()
      expect(screen.getByText('(1h 1m)')).toBeInTheDocument()
    })

    it('prepends ticket ID with middle dot for ticket-scoped agent', () => {
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText(/ticket-1 · feature \/ implementor/)).toBeInTheDocument()
    })

    it('does not show ticket ID prefix or separator for project-scoped agent', () => {
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      // project-scoped agent (empty ticket_id) — no middle dot before workflow
      expect(screen.queryByText(/·.*bugfix/)).not.toBeInTheDocument()
    })

    it('ticket-scoped agent link points to /tickets/{id}', () => {
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      const links = screen.getAllByRole('link')
      expect(links.find((l) => l.getAttribute('href') === '/tickets/ticket-1?tab=workflow')).toBeDefined()
    })

    it('project-scoped agent link points to /project-workflows', () => {
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      const links = screen.getAllByRole('link')
      expect(links.find((l) => l.getAttribute('href') === '/project-workflows')).toBeDefined()
    })

    it('popover stays visible when mouse enters popover', () => {
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText('Running Agents (2)')).toBeInTheDocument()

      // Move mouse to trigger's leave, then to popover — popover should stay
      fireEvent.mouseLeave(getTrigger())
      const popover = screen.getByText('Running Agents (2)').closest('div')!.parentElement!
      fireEvent.mouseEnter(popover)
      expect(screen.getByText('Running Agents (2)')).toBeInTheDocument()
    })
  })

  describe('navigation on click', () => {
    it('navigates to ticket page and skips setCurrentProject for same project', () => {
      mockRunningAgents = {
        data: { agents: [makeAgent({ ticket_id: 'ticket-42', project_id: 'proj-1' })], count: 1 },
      }
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      fireEvent.click(screen.getByRole('link'))
      expect(mockNavigate).toHaveBeenCalledWith('/tickets/ticket-42?tab=workflow')
      expect(mockSetCurrentProject).not.toHaveBeenCalled()
    })

    it('navigates to /project-workflows for project-scoped agent', () => {
      mockRunningAgents = {
        data: { agents: [makeAgent({ ticket_id: '', project_id: 'proj-1' })], count: 1 },
      }
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      fireEvent.click(screen.getByRole('link'))
      expect(mockNavigate).toHaveBeenCalledWith('/project-workflows')
    })

    it('calls setCurrentProject before navigate for agent in different project', () => {
      mockRunningAgents = {
        data: { agents: [makeAgent({ project_id: 'proj-2', ticket_id: 'ticket-5' })], count: 1 },
      }
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      fireEvent.click(screen.getByRole('link'))
      expect(mockSetCurrentProject).toHaveBeenCalledWith('proj-2')
      expect(mockNavigate).toHaveBeenCalledWith('/tickets/ticket-5?tab=workflow')
    })
  })

  describe('formatAgentLabel', () => {
    it('shows "Merge Conflict Resolver" for conflict-resolver agent type', () => {
      mockRunningAgents = {
        data: {
          agents: [makeAgent({ agent_type: 'conflict-resolver', workflow_id: '_conflict_resolution' })],
          count: 1,
        },
      }
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText(/Merge Conflict Resolver/)).toBeInTheDocument()
    })

    it('shows workflow_id / agent_type for normal agents', () => {
      mockRunningAgents = {
        data: { agents: [makeAgent({ workflow_id: 'feature', agent_type: 'implementor' })], count: 1 },
      }
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
      expect(screen.getByText(/feature \/ implementor/)).toBeInTheDocument()
    })
  })

  describe('formatElapsed', () => {
    function renderAndHover(elapsed_sec: number) {
      mockRunningAgents = { data: { agents: [makeAgent({ elapsed_sec })], count: 1 } }
      renderIndicator()
      fireEvent.mouseEnter(getTrigger())
    }

    it('formats seconds only (30s)', () => {
      renderAndHover(30)
      expect(screen.getByText('(30s)')).toBeInTheDocument()
    })

    it('formats minutes and seconds (1m 30s)', () => {
      renderAndHover(90)
      expect(screen.getByText('(1m 30s)')).toBeInTheDocument()
    })

    it('formats whole minutes without seconds (2m)', () => {
      renderAndHover(120)
      expect(screen.getByText('(2m)')).toBeInTheDocument()
    })

    it('formats hours and minutes (1h 1m)', () => {
      renderAndHover(3661)
      expect(screen.getByText('(1h 1m)')).toBeInTheDocument()
    })

    it('formats whole hours without minutes (1h)', () => {
      renderAndHover(3600)
      expect(screen.getByText('(1h)')).toBeInTheDocument()
    })
  })
})
