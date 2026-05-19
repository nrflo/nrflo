import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RunningAgentsIndicator } from './RunningAgentsIndicator'
import type { RunningAgent, RunningAgentsResponse } from '@/types/agents'

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return { ...actual, useNavigate: () => vi.fn() }
})

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: Object.assign(
    (selector: (s: { currentProject: string }) => unknown) =>
      selector({ currentProject: 'proj-1' }),
    { getState: () => ({ setCurrentProject: vi.fn() }) },
  ),
}))

let mockRunningAgents: { data: RunningAgentsResponse | undefined } = { data: undefined }
vi.mock('@/hooks/useRunningAgents', () => ({
  useRunningAgents: () => mockRunningAgents,
}))

const BASE_TIME = new Date('2026-01-01T00:00:00.000Z')

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
    elapsed_sec: 90,
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

describe('RunningAgentsIndicator — rate-limited agents', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(BASE_TIME)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
    mockRunningAgents = { data: undefined }
  })

  it('shows "waiting · resumes HH:MM:SS" countdown for rate-limited agent in popover', () => {
    const untilTs = new Date(BASE_TIME.getTime() + 65 * 1000).toISOString()
    mockRunningAgents = {
      data: {
        agents: [makeAgent({ waiting_for_rate_limit: true, rate_limit_until_ts: untilTs })],
        count: 1,
      },
    }
    renderIndicator()
    fireEvent.mouseEnter(getTrigger())
    expect(screen.getByText(/waiting · resumes 1m 5s/)).toBeInTheDocument()
  })

  it('shows "waiting · resumes soon" when rate_limit_until_ts is absent', () => {
    mockRunningAgents = {
      data: {
        agents: [makeAgent({ waiting_for_rate_limit: true, rate_limit_until_ts: undefined })],
        count: 1,
      },
    }
    renderIndicator()
    fireEvent.mouseEnter(getTrigger())
    expect(screen.getByText(/waiting · resumes soon/)).toBeInTheDocument()
  })

  it('does not show elapsed time for rate-limited agent', () => {
    const untilTs = new Date(BASE_TIME.getTime() + 30 * 1000).toISOString()
    mockRunningAgents = {
      data: {
        agents: [makeAgent({ waiting_for_rate_limit: true, rate_limit_until_ts: untilTs, elapsed_sec: 90 })],
        count: 1,
      },
    }
    renderIndicator()
    fireEvent.mouseEnter(getTrigger())
    // elapsed row (1m 30s) must NOT appear; only the waiting countdown
    expect(screen.queryByText('(1m 30s)')).not.toBeInTheDocument()
    expect(screen.getByText(/waiting · resumes/)).toBeInTheDocument()
  })

  it('still shows elapsed time for non-rate-limited agent', () => {
    mockRunningAgents = {
      data: {
        agents: [makeAgent({ waiting_for_rate_limit: false, elapsed_sec: 90 })],
        count: 1,
      },
    }
    renderIndicator()
    fireEvent.mouseEnter(getTrigger())
    expect(screen.getByText('(1m 30s)')).toBeInTheDocument()
    expect(screen.queryByText(/waiting · resumes/)).not.toBeInTheDocument()
  })
})
