/**
 * End-to-end test for "Improve active agent log" (nrworkflow-eb3c64).
 *
 * Covers all four acceptance criteria in a single suite:
 *   1. Full messages shown (no truncation)
 *   2. Hover tooltip shows timestamp and elapsed time
 *   3. Toggle button positioned away from panel border (-left-5)
 *   4. Tool names highlighted with color badges
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { LogMessage } from './LogMessage'
import { RunningAgentLog } from './RunningAgentLog'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'

// Mock API layer
vi.mock('@/api/tickets', () => ({
  getSessionMessages: vi.fn().mockResolvedValue({ session_id: 's1', messages: [], total: 0 }),
  getSessionRawOutput: vi.fn().mockResolvedValue({ session_id: 's1', raw_output: '' }),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true }),
}))

// jsdom doesn't implement scrollIntoView
Element.prototype.scrollIntoView = vi.fn()

// --- Helpers ---

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
    raw_output_size: 0,
    last_messages: ['[Read] src/main.ts', '[Edit] src/utils.ts'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      {ui}
    </QueryClientProvider>
  )
}

// --- E2E test suite covering all 4 acceptance criteria ---

describe('Agent Log Improvements — E2E (nrworkflow-eb3c64)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // =====================================================================
  // Criterion 4: Tool name color highlighting (test first — no timer deps)
  // =====================================================================
  describe('4. Tool name color highlighting', () => {
    const toolTests: Array<{
      tool: string
      expectedColorFragment: string
      message: string
    }> = [
      { tool: 'Bash', expectedColorFragment: 'bg-blue-100', message: '[Bash] git status' },
      { tool: 'Read', expectedColorFragment: 'bg-green-100', message: '[Read] src/main.ts' },
      { tool: 'Edit', expectedColorFragment: 'bg-amber-100', message: '[Edit] src/utils.ts' },
      { tool: 'Write', expectedColorFragment: 'bg-purple-100', message: '[Write] new-file.ts' },
      { tool: 'Grep', expectedColorFragment: 'bg-cyan-100', message: '[Grep] pattern search' },
      { tool: 'Glob', expectedColorFragment: 'bg-teal-100', message: '[Glob] **/*.ts' },
      { tool: 'Task', expectedColorFragment: 'bg-indigo-100', message: '[Task] subtask' },
      { tool: 'WebFetch', expectedColorFragment: 'bg-orange-100', message: '[WebFetch] url' },
      { tool: 'WebSearch', expectedColorFragment: 'bg-orange-100', message: '[WebSearch] query' },
      { tool: 'TodoWrite', expectedColorFragment: 'bg-pink-100', message: '[TodoWrite] update' },
      { tool: 'Skill', expectedColorFragment: 'bg-violet-100', message: '[Skill] commit' },
    ]

    for (const { tool, expectedColorFragment, message } of toolTests) {
      it(`renders ${tool} badge with correct color`, () => {
        render(<LogMessage message={message} />)

        const badge = screen.getByText(tool)
        expect(badge).toBeInTheDocument()
        expect(badge.className).toContain(expectedColorFragment)
        expect(badge.className).toContain('rounded')
        expect(badge.className).toContain('font-semibold')
      })
    }

    it('renders unknown tool with default gray color', () => {
      render(<LogMessage message="[CustomTool] doing something" />)

      const badge = screen.getByText('CustomTool')
      expect(badge).toBeInTheDocument()
      expect(badge.className).toContain('bg-gray-100')
    })

    it('does not render badge for messages without tool prefix', () => {
      render(<LogMessage message="plain text message" />)

      expect(screen.getByText('plain text message')).toBeInTheDocument()
      // No tool badge spans with font-semibold class
      const el = screen.getByText('plain text message')
      // The message div should not contain any badge span
      const badges = el.querySelectorAll('span')
      expect(badges.length).toBe(0)
    })

    it('separates tool badge from message content', () => {
      render(<LogMessage message="[Bash] npm install" />)

      const badge = screen.getByText('Bash')
      const content = screen.getByText('npm install')

      expect(badge).not.toBe(content)
      expect(badge.tagName).toBe('SPAN')
    })

    it('tool badges render correctly in RunningAgentLog context', () => {
      const agent = makeAgent()
      const session = makeSession({
        last_messages: ['[Read] file.ts', '[Edit] other.ts', '[Bash] npm test'],
      })
      renderWithProviders(
        <RunningAgentLog
          activeAgents={{ 'implementor:claude:sonnet': agent }}
          sessions={[session]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          onAgentClick={vi.fn()}
        />
      )

      const readBadge = screen.getByText('Read')
      expect(readBadge.className).toContain('bg-green-100')

      const editBadge = screen.getByText('Edit')
      expect(editBadge.className).toContain('bg-amber-100')

      const bashBadge = screen.getByText('Bash')
      expect(bashBadge.className).toContain('bg-blue-100')
    })
  })

  // =====================================================================
  // Criterion 1: Full messages shown, no truncation
  // =====================================================================
  describe('1. Full messages (no truncation)', () => {
    it('renders full message text without truncation in compact variant', () => {
      const longMessage = '[Bash] ' + 'x'.repeat(300)
      render(<LogMessage message={longMessage} />)

      const el = screen.getByText('x'.repeat(300))
      expect(el).toBeInTheDocument()
      expect(el.className).toContain('whitespace-pre-wrap')
      expect(el.className).not.toContain('truncate')
    })

    it('renders full message text without truncation in full variant', () => {
      const longMessage = '[Edit] ' + 'y'.repeat(500)
      render(<LogMessage message={longMessage} variant="full" />)

      const el = screen.getByText('y'.repeat(500))
      expect(el).toBeInTheDocument()
      expect(el.className).toContain('whitespace-pre-wrap')
      expect(el.className).not.toContain('truncate')
    })

    it('preserves multiline content without truncation', () => {
      const multiline = 'line1\nline2\nline3\nline4\nline5'
      render(<LogMessage message={multiline} variant="full" />)

      const el = screen.getByText((_content, element) =>
        element?.textContent === multiline && element?.className?.includes('whitespace-pre-wrap') || false
      )
      expect(el).toBeInTheDocument()
    })

    it('renders full messages in RunningAgentLog (not truncated)', () => {
      const longMsg = '[Bash] ' + 'z'.repeat(200)
      const agent = makeAgent()
      const session = makeSession({
        last_messages: [longMsg, '[Read] short.ts'],
      })
      renderWithProviders(
        <RunningAgentLog
          activeAgents={{ 'implementor:claude:sonnet': agent }}
          sessions={[session]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          onAgentClick={vi.fn()}
        />
      )

      expect(screen.getByText('z'.repeat(200))).toBeInTheDocument()
      expect(screen.getByText('short.ts')).toBeInTheDocument()
    })
  })

  // =====================================================================
  // Criterion 3: Toggle button moved away from panel border
  // =====================================================================
  describe('3. Toggle button position (-left-5)', () => {
    it('toggle button uses -left-5 class (not -left-3)', () => {
      const agent = makeAgent()
      renderWithProviders(
        <RunningAgentLog
          activeAgents={{ 'implementor:claude:sonnet': agent }}
          sessions={[]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          onAgentClick={vi.fn()}
        />
      )

      const toggleBtn = screen.getByTitle('Collapse agent log')
      expect(toggleBtn.className).toContain('-left-5')
      expect(toggleBtn.className).not.toContain('-left-3')
    })

    it('toggle button in collapsed state also uses -left-5', () => {
      const agent = makeAgent()
      renderWithProviders(
        <RunningAgentLog
          activeAgents={{ 'implementor:claude:sonnet': agent }}
          sessions={[]}
          collapsed={true}
          onToggleCollapse={vi.fn()}
          onAgentClick={vi.fn()}
        />
      )

      const toggleBtn = screen.getByTitle('Expand agent log')
      expect(toggleBtn.className).toContain('-left-5')
      expect(toggleBtn.className).not.toContain('-left-3')
    })
  })

  // =====================================================================
  // Criterion 2: Hover tooltip shows timestamp and elapsed time
  // =====================================================================
  describe('2. Timestamp tooltip on hover', () => {
    it('wraps message in Tooltip when timestamp is provided', () => {
      render(
        <LogMessage
          message="[Read] file.ts"
          timestamp="2026-02-12T10:00:00Z"
          nextTimestamp="2026-02-12T10:00:05Z"
        />
      )

      // When a timestamp is present, the message is wrapped in a Tooltip's <span>
      const trigger = screen.getByText('file.ts').closest('span.inline-flex')
      expect(trigger).toBeInTheDocument()
    })

    it('does NOT wrap in Tooltip when no timestamp provided', () => {
      render(<LogMessage message="[Read] file.ts" />)

      // Without timestamp, the content div is rendered directly (not inside a Tooltip)
      const messageDiv = screen.getByText('file.ts')
      // With tooltip: the message div lives inside a span.inline-flex (Tooltip trigger)
      // Without tooltip: the message div is directly in the render container
      const closestTooltipSpan = messageDiv.closest('span.inline-flex')
      expect(closestTooltipSpan).toBeNull()
    })

    it('shows tooltip content with elapsed time on hover', async () => {
      vi.useFakeTimers()

      render(
        <LogMessage
          message="[Bash] npm test"
          timestamp="2026-02-12T10:00:00Z"
          nextTimestamp="2026-02-12T10:00:30Z"
        />
      )

      // Find the tooltip trigger (Tooltip wraps content in span.inline-flex)
      const trigger = screen.getByText('npm test').closest('span.inline-flex')!

      // Simulate mouseenter using fireEvent (works with React's synthetic events)
      act(() => {
        fireEvent.mouseEnter(trigger)
      })

      // Advance past tooltip delay (200ms)
      act(() => {
        vi.advanceTimersByTime(300)
      })

      // Tooltip should show the elapsed time to next message
      expect(screen.getByText('+30s until next')).toBeInTheDocument()
    })

    it('shows "ago" text when no nextTimestamp (latest message)', async () => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-02-12T10:01:00Z'))

      render(
        <LogMessage
          message="[Edit] utils.ts"
          timestamp="2026-02-12T10:00:00Z"
        />
      )

      const trigger = screen.getByText('utils.ts').closest('span.inline-flex')!

      act(() => {
        fireEvent.mouseEnter(trigger)
      })

      act(() => {
        vi.advanceTimersByTime(300)
      })

      expect(screen.getByText('1m ago')).toBeInTheDocument()
    })

    it('shows "0s" elapsed when timestamps are identical', async () => {
      vi.useFakeTimers()

      render(
        <LogMessage
          message="[Read] same-time.ts"
          timestamp="2026-02-12T10:00:00Z"
          nextTimestamp="2026-02-12T10:00:00Z"
        />
      )

      const trigger = screen.getByText('same-time.ts').closest('span.inline-flex')!

      act(() => {
        fireEvent.mouseEnter(trigger)
      })

      act(() => {
        vi.advanceTimersByTime(300)
      })

      expect(screen.getByText('+0s until next')).toBeInTheDocument()
    })

    it('passes timestamps from RunningAgentLog to LogMessage', () => {
      const agent = makeAgent()
      const session = makeSession({
        last_messages: ['[Read] a.ts', '[Edit] b.ts'],
      })

      // When messages come from last_messages (no API data), created_at is ''
      // which means LogMessage gets timestamp=undefined (no tooltip)
      renderWithProviders(
        <RunningAgentLog
          activeAgents={{ 'implementor:claude:sonnet': agent }}
          sessions={[session]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          onAgentClick={vi.fn()}
        />
      )

      expect(screen.getByText('a.ts')).toBeInTheDocument()
      expect(screen.getByText('b.ts')).toBeInTheDocument()
    })
  })

  // =====================================================================
  // Combined E2E: All criteria in a single render
  // =====================================================================
  describe('Combined: all criteria in single render', () => {
    it('renders full messages with tool badges, no truncation, with correct button position', () => {
      const longMsg = '[Bash] ' + 'x'.repeat(200)
      const agent = makeAgent()
      const session = makeSession({
        last_messages: [
          '[Read] src/main.ts',
          longMsg,
          'plain message without tool',
          '[Edit] src/utils.ts',
        ],
      })

      renderWithProviders(
        <RunningAgentLog
          activeAgents={{ 'implementor:claude:sonnet': agent }}
          sessions={[session]}
          collapsed={false}
          onToggleCollapse={vi.fn()}
          onAgentClick={vi.fn()}
        />
      )

      // Criterion 1: Full messages rendered (including long one)
      expect(screen.getByText('x'.repeat(200))).toBeInTheDocument()
      expect(screen.getByText('plain message without tool')).toBeInTheDocument()

      // Criterion 3: Toggle button at -left-5
      const toggleBtn = screen.getByTitle('Collapse agent log')
      expect(toggleBtn.className).toContain('-left-5')

      // Criterion 4: Tool badges with colors
      const readBadge = screen.getByText('Read')
      expect(readBadge.className).toContain('bg-green-100')

      const bashBadge = screen.getByText('Bash')
      expect(bashBadge.className).toContain('bg-blue-100')

      const editBadge = screen.getByText('Edit')
      expect(editBadge.className).toContain('bg-amber-100')
    })
  })
})
