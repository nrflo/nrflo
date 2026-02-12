import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RunningAgentLog } from './RunningAgentLog'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'

vi.mock('@/api/tickets', () => ({
  getSessionMessages: vi.fn().mockResolvedValue({ session_id: 's1', messages: [], total: 0 }),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true }),
}))

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    model: 'sonnet',
    pid: 12345,
    session_id: 's1',
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 's1',
    project_id: 'proj1',
    ticket_id: 'T-1',
    workflow_instance_id: 'wi1',
    phase: 'implementation',
    workflow: 'feature',
    agent_type: 'implementor',
    model_id: 'claude-sonnet-4-5',
    status: 'running',
    message_count: 5,
    last_messages: ['[Read] src/main.ts', '[Edit] src/utils.ts'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderLog(props: {
  activeAgents?: Record<string, ActiveAgentV4>
  sessions?: AgentSession[]
  collapsed?: boolean
  onToggleCollapse?: () => void
  onAgentClick?: (agent: ActiveAgentV4, session?: AgentSession) => void
}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const defaultProps = {
    activeAgents: props.activeAgents ?? {},
    sessions: props.sessions ?? [],
    collapsed: props.collapsed ?? false,
    onToggleCollapse: props.onToggleCollapse ?? vi.fn(),
    onAgentClick: props.onAgentClick ?? vi.fn(),
  }
  return render(
    <QueryClientProvider client={queryClient}>
      <RunningAgentLog {...defaultProps} />
    </QueryClientProvider>
  )
}

describe('RunningAgentLog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when no active agents', () => {
    const { container } = renderLog({ activeAgents: {} })
    expect(container.innerHTML).toBe('')
  })

  it('renders nothing when all agents have a result (finished)', () => {
    const finishedAgent = makeAgent({ result: 'pass' })
    const { container } = renderLog({
      activeAgents: { 'implementor:claude:sonnet': finishedAgent },
    })
    expect(container.innerHTML).toBe('')
  })

  it('renders panel with running agent count when agents are active', () => {
    const agent = makeAgent()
    renderLog({
      activeAgents: { 'implementor:claude:sonnet': agent },
      sessions: [makeSession()],
    })
    expect(screen.getByText('Running Agents (1)')).toBeInTheDocument()
  })

  it('shows correct count for multiple running agents', () => {
    const agent1 = makeAgent({ agent_id: 'a1', agent_type: 'implementor', phase: 'implementation' })
    const agent2 = makeAgent({ agent_id: 'a2', agent_type: 'reviewer', phase: 'implementation', model_id: 'claude-opus-4' })
    renderLog({
      activeAgents: {
        'implementor:claude:sonnet': agent1,
        'reviewer:claude:opus': agent2,
      },
    })
    expect(screen.getByText('Running Agents (2)')).toBeInTheDocument()
  })

  it('excludes finished agents from the count', () => {
    const running = makeAgent({ agent_id: 'a1', agent_type: 'implementor' })
    const finished = makeAgent({ agent_id: 'a2', agent_type: 'reviewer', result: 'pass' })
    renderLog({
      activeAgents: {
        'implementor:claude:sonnet': running,
        'reviewer:claude:opus': finished,
      },
    })
    expect(screen.getByText('Running Agents (1)')).toBeInTheDocument()
  })

  it('displays agent phase and model name', () => {
    const agent = makeAgent({ phase: 'investigation', model_id: 'claude-sonnet-4-5' })
    renderLog({
      activeAgents: { 'agent:claude:sonnet': agent },
      sessions: [makeSession({ phase: 'investigation' })],
    })
    // Phase and model rendered inside same span: "investigation — 4-5"
    // model_id.split('-').slice(-2).join('-') => "4-5"
    const span = screen.getByText((content, element) =>
      element?.tagName === 'SPAN' &&
      element?.className.includes('truncate') &&
      content.includes('investigation') || false
    )
    expect(span).toBeInTheDocument()
    expect(span.textContent).toContain('investigation')
    expect(span.textContent).toContain('4-5')
  })

  it('calls onAgentClick when agent row is clicked', async () => {
    const user = userEvent.setup()
    const onAgentClick = vi.fn()
    const agent = makeAgent()
    const session = makeSession()
    renderLog({
      activeAgents: { 'implementor:claude:sonnet': agent },
      sessions: [session],
      onAgentClick,
    })

    // Find the agent row button (contains truncate span with phase name)
    const span = screen.getByText((_content, element) =>
      element?.tagName === 'SPAN' &&
      element?.className.includes('truncate') &&
      !!element?.textContent?.includes('implementation') || false
    )
    await user.click(span.closest('button')!)

    expect(onAgentClick).toHaveBeenCalledTimes(1)
    expect(onAgentClick).toHaveBeenCalledWith(agent, session)
  })

  it('shows collapsed state with agent count badge', () => {
    const agent = makeAgent()
    renderLog({
      activeAgents: { 'implementor:claude:sonnet': agent },
      collapsed: true,
    })

    // Should not show expanded header
    expect(screen.queryByText('Running Agents (1)')).not.toBeInTheDocument()
    // Should show collapsed count badge
    expect(screen.getByText('1')).toBeInTheDocument()
    // Should show vertical label
    expect(screen.getByText('Agent Log')).toBeInTheDocument()
  })

  it('calls onToggleCollapse when toggle button is clicked', async () => {
    const user = userEvent.setup()
    const onToggleCollapse = vi.fn()
    const agent = makeAgent()
    renderLog({
      activeAgents: { 'implementor:claude:sonnet': agent },
      onToggleCollapse,
    })

    const toggleBtn = screen.getByTitle('Collapse agent log')
    await user.click(toggleBtn)

    expect(onToggleCollapse).toHaveBeenCalledTimes(1)
  })

  it('shows expand title when collapsed', () => {
    const agent = makeAgent()
    renderLog({
      activeAgents: { 'implementor:claude:sonnet': agent },
      collapsed: true,
    })
    expect(screen.getByTitle('Expand agent log')).toBeInTheDocument()
  })

  it('matches session by agent_type, phase, and model_id', async () => {
    const user = userEvent.setup()
    const onAgentClick = vi.fn()
    const agent = makeAgent({
      agent_type: 'implementor',
      phase: 'implementation',
      model_id: 'claude-sonnet-4-5',
    })
    const matchingSession = makeSession({
      id: 's-match',
      agent_type: 'implementor',
      phase: 'implementation',
      model_id: 'claude-sonnet-4-5',
    })
    const otherSession = makeSession({
      id: 's-other',
      agent_type: 'reviewer',
      phase: 'verification',
      model_id: 'claude-opus-4',
    })

    renderLog({
      activeAgents: { 'implementor:claude:sonnet': agent },
      sessions: [otherSession, matchingSession],
      onAgentClick,
    })

    const span = screen.getByText((_content, element) =>
      element?.tagName === 'SPAN' &&
      element?.className.includes('truncate') &&
      !!element?.textContent?.includes('implementation') || false
    )
    await user.click(span.closest('button')!)

    expect(onAgentClick).toHaveBeenCalledWith(agent, matchingSession)
  })

  it('passes undefined session when no session matches', async () => {
    const user = userEvent.setup()
    const onAgentClick = vi.fn()
    const agent = makeAgent({ agent_type: 'implementor', phase: 'implementation' })
    const nonMatchingSession = makeSession({
      agent_type: 'reviewer',
      phase: 'verification',
    })

    renderLog({
      activeAgents: { 'implementor:claude:sonnet': agent },
      sessions: [nonMatchingSession],
      onAgentClick,
    })

    const span = screen.getByText((_content, element) =>
      element?.tagName === 'SPAN' &&
      element?.className.includes('truncate') &&
      !!element?.textContent?.includes('implementation') || false
    )
    await user.click(span.closest('button')!)

    expect(onAgentClick).toHaveBeenCalledWith(agent, undefined)
  })

  it('falls back to cli for model name when model_id is absent', () => {
    const agent = makeAgent({ model_id: undefined, cli: 'opencode' })
    renderLog({
      activeAgents: { 'agent:opencode': agent },
    })
    const span = screen.getByText((_content, element) =>
      element?.tagName === 'SPAN' &&
      element?.className.includes('truncate') &&
      !!element?.textContent?.includes('opencode') || false
    )
    expect(span.textContent).toContain('implementation')
    expect(span.textContent).toContain('opencode')
  })

  it('falls back to agent_type for model name when model_id and cli are absent', () => {
    const agent = makeAgent({ model_id: undefined, cli: undefined, agent_type: 'test-agent' })
    renderLog({
      activeAgents: { 'agent:test': agent },
    })
    const span = screen.getByText((_content, element) =>
      element?.tagName === 'SPAN' &&
      element?.className.includes('truncate') &&
      !!element?.textContent?.includes('test-agent') || false
    )
    expect(span.textContent).toContain('implementation')
    expect(span.textContent).toContain('test-agent')
  })

  describe('layout changes', () => {
    it('expanded state uses flex-1 min-w-[300px] instead of fixed width', () => {
      const agent = makeAgent()
      const { container } = renderLog({
        activeAgents: { 'implementor:claude:sonnet': agent },
        collapsed: false,
      })
      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('flex-1')
      expect(panel.className).toContain('min-w-[300px]')
      expect(panel.className).not.toContain('w-[380px]')
    })

    it('collapsed state uses w-10 (narrow strip)', () => {
      const agent = makeAgent()
      const { container } = renderLog({
        activeAgents: { 'implementor:claude:sonnet': agent },
        collapsed: true,
      })
      const panel = container.firstChild as HTMLElement
      expect(panel.className).toContain('w-10')
      expect(panel.className).not.toContain('flex-1')
    })

    it('collapsed state has pt-16 for spacing from toggle button', () => {
      const agent = makeAgent()
      renderLog({
        activeAgents: { 'implementor:claude:sonnet': agent },
        collapsed: true,
      })
      // The collapsed inner div with the count badge and "Agent Log" label
      const agentLogLabel = screen.getByText('Agent Log')
      const collapsedContainer = agentLogLabel.closest('div.flex.flex-col') as HTMLElement
      expect(collapsedContainer.className).toContain('pt-16')
    })

    it('expanded messages area uses flex-1 overflow-y-auto (no max-h constraint)', () => {
      const agent = makeAgent()
      const session = makeSession()
      const { container } = renderLog({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [session],
        collapsed: false,
      })
      // The scrollable messages area has ref={scrollRef} and is the flex-1 overflow-y-auto div
      const scrollArea = container.querySelector('.flex-1.overflow-y-auto')
      expect(scrollArea).toBeInTheDocument()
      // Ensure no max-h-48 constraint
      expect(scrollArea?.className).not.toContain('max-h-48')
    })

    it('renders messages using LogMessage component with compact variant', () => {
      const agent = makeAgent()
      const session = makeSession({
        last_messages: ['[Read] file.ts', '[Edit] other.ts'],
      })
      renderLog({
        activeAgents: { 'implementor:claude:sonnet': agent },
        sessions: [session],
        collapsed: false,
      })
      // LogMessage compact renders tool badges and message text
      expect(screen.getByText('file.ts')).toBeInTheDocument()
      expect(screen.getByText('other.ts')).toBeInTheDocument()
      // Tool badges rendered separately
      expect(screen.getByText('Read')).toBeInTheDocument()
      expect(screen.getByText('Edit')).toBeInTheDocument()
    })
  })
})
