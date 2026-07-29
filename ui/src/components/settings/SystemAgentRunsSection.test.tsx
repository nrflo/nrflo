import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act } from '@testing-library/react'
import { screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { SystemAgentRunsSection } from './SystemAgentRunsSection'
import * as systemAgentRunsApi from '@/api/systemAgentRuns'
import { renderWithQuery } from '@/test/utils'
import type { SystemAgentRun } from '@/types/systemAgentRuns'
import type { WSEvent } from '@/hooks/useWebSocket'

vi.mock('@/api/systemAgentRuns')
vi.mock('@/api/handoffDigest', () => ({
  fetchSessionHandoffDigest: vi.fn().mockResolvedValue(null),
}))

let capturedHandler: ((e: WSEvent) => void) | null = null
const mockAddEventListener = vi.fn((fn: (e: WSEvent) => void) => {
  capturedHandler = fn
})
const mockRemoveEventListener = vi.fn()

vi.mock('@/providers/WebSocketProvider', () => ({
  useWebSocketContext: () => ({
    addEventListener: mockAddEventListener,
    removeEventListener: mockRemoveEventListener,
  }),
}))

function makeSessionRun(overrides: Partial<SystemAgentRun> = {}): SystemAgentRun {
  return {
    kind: 'agent_session',
    session_id: 's1',
    agent_type: 'implementor',
    tier: 2,
    resolved_provider: 'local',
    resolved_execution_mode: 'cli_interactive',
    resolved_effort: 'medium',
    chain_position: 0,
    model_id: 'qwen3-local',
    tokens_json: { input_tokens: 100, output_tokens: 50 },
    cost_estimate: 0.01,
    status: 'completed',
    result: 'completed',
    ticket_id: 'ticket-abc-123',
    project_id: 'proj-1',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeFoldRun(overrides: Partial<SystemAgentRun> = {}): SystemAgentRun {
  return {
    kind: 'refinery_fold',
    session_id: 's1',
    agent_type: '_refinery',
    provider: 'anthropic',
    model_id: 'sonnet-5',
    prompt_tokens: 200,
    output_tokens: 30,
    status: 'failed',
    error: 'no api key',
    created_at: '2026-01-01T00:01:00Z',
    ...overrides,
  } as SystemAgentRun
}

function makeStepRotationRun(overrides: Partial<SystemAgentRun> = {}): SystemAgentRun {
  return {
    kind: 'step_rotation',
    session_id: 'sr1',
    step_id: 'step-3',
    node_id: 'implementation',
    workflow_instance_id: 'wi1',
    status: 'rotated',
    created_at: '2026-01-01T00:02:00Z',
    ...overrides,
  } as SystemAgentRun
}

function renderSection() {
  return renderWithQuery(
    <MemoryRouter>
      <SystemAgentRunsSection />
    </MemoryRouter>
  )
}

describe('SystemAgentRunsSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    capturedHandler = null
  })

  it('renders the fallback badge only for a fallback session, and inline refinery-fold rows with their status/error', async () => {
    vi.mocked(systemAgentRunsApi.listSystemAgentRuns).mockResolvedValue({
      items: [
        makeSessionRun({ session_id: 'no-fallback', chain_position: 0 }),
        makeSessionRun({
          session_id: 'with-fallback',
          chain_position: 1,
          model_id: 'qwen3-local',
          fallback_from: [{ provider: 'anthropic', model_id: 'sonnet-5', execution_mode: 'api', reasoning_effort: '', tier: 1 }],
        }),
        makeFoldRun({ status: 'failed', error: 'no api key' }),
        makeFoldRun({ session_id: 's2', status: 'ok', error: undefined }),
      ],
      limit: 50,
    })

    renderSection()

    expect(await screen.findByText('sonnet-5 → qwen3-local')).toBeInTheDocument()
    // Two session rows total; only one carries the fallback badge.
    expect(screen.getAllByText('implementor')).toHaveLength(2)

    // Refinery fold rows render inline with the merged table, no tier badge.
    expect(screen.getAllByText('Refinery fold')).toHaveLength(2)
    expect(screen.getByText('no api key')).toBeInTheDocument()
  })

  it('renders a step_rotation row with its label, zero token counts, a secondary badge, and no expand chevron', async () => {
    vi.mocked(systemAgentRunsApi.listSystemAgentRuns).mockResolvedValue({
      items: [makeStepRotationRun()],
      limit: 50,
    })

    renderSection()

    expect(await screen.findByText('Step rotation (step-3)')).toBeInTheDocument()
    expect(screen.getByText('0 in / 0 out')).toBeInTheDocument()
    const statusBadge = screen.getByText('rotated')
    expect(statusBadge.className).toContain('bg-secondary')
    expect(screen.queryByRole('button', { name: /expand|collapse/i })).not.toBeInTheDocument()
  })

  it('invalidates and refetches on a step.advanced event with rotated: true, and not on rotated: false', async () => {
    vi.mocked(systemAgentRunsApi.listSystemAgentRuns).mockResolvedValueOnce({
      items: [makeSessionRun({ session_id: 'initial' })],
      limit: 50,
    })

    const { queryClient } = renderSection()
    await screen.findByText('implementor')

    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    act(() => {
      capturedHandler?.({
        type: 'step.advanced',
        project_id: 'p',
        ticket_id: '',
        timestamp: '2026-01-01T00:02:00Z',
        data: { rotated: false },
      })
    })
    expect(invalidateSpy).not.toHaveBeenCalled()

    vi.mocked(systemAgentRunsApi.listSystemAgentRuns).mockResolvedValueOnce({
      items: [makeStepRotationRun()],
      limit: 50,
    })

    act(() => {
      capturedHandler?.({
        type: 'step.advanced',
        project_id: 'p',
        ticket_id: '',
        timestamp: '2026-01-01T00:03:00Z',
        data: { rotated: true },
      })
    })

    expect(invalidateSpy).toHaveBeenCalled()
    expect(await screen.findByText('Step rotation (step-3)')).toBeInTheDocument()
  })

  it('invalidates and refetches on agent.handoff_digest and refinery.fold_failed WS events', async () => {
    vi.mocked(systemAgentRunsApi.listSystemAgentRuns).mockResolvedValueOnce({
      items: [makeSessionRun({ session_id: 'initial' })],
      limit: 50,
    })

    const { queryClient } = renderSection()
    await screen.findByText('implementor')

    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    vi.mocked(systemAgentRunsApi.listSystemAgentRuns).mockResolvedValueOnce({
      items: [makeFoldRun({ session_id: 'new-fold', status: 'failed', error: 'no api key' })],
      limit: 50,
    })

    act(() => {
      capturedHandler?.({
        type: 'refinery.fold_failed',
        project_id: 'p',
        ticket_id: '',
        timestamp: '2026-01-01T00:02:00Z',
        data: {},
      })
    })

    expect(invalidateSpy).toHaveBeenCalled()
    expect(await screen.findByText('no api key')).toBeInTheDocument()

    vi.mocked(systemAgentRunsApi.listSystemAgentRuns).mockResolvedValueOnce({
      items: [makeSessionRun({ session_id: 'after-digest', agent_type: 'qa-verifier' })],
      limit: 50,
    })

    act(() => {
      capturedHandler?.({
        type: 'agent.handoff_digest',
        project_id: 'p',
        ticket_id: '',
        timestamp: '2026-01-01T00:03:00Z',
        data: {},
      })
    })

    expect(await screen.findByText('qa-verifier')).toBeInTheDocument()
  })

  it('renders a delegation group for workers sharing a delegation_id alongside ungrouped rows', async () => {
    vi.mocked(systemAgentRunsApi.listSystemAgentRuns).mockResolvedValue({
      items: [
        makeSessionRun({ session_id: 'standalone', agent_type: 'qa-verifier' }),
        makeSessionRun({
          session_id: 'worker-1',
          agent_type: 'executor',
          delegation_id: 'delegation-1',
          caller_session_id: 'caller-1',
          delegate_tier: 'executor',
          fanout: 2,
        }),
        makeSessionRun({
          session_id: 'worker-2',
          agent_type: 'executor',
          delegation_id: 'delegation-1',
          caller_session_id: 'caller-1',
          delegate_tier: 'executor',
          fanout: 2,
        }),
      ],
      limit: 50,
    })

    renderSection()

    expect(await screen.findByText('qa-verifier')).toBeInTheDocument()
    expect(screen.getByText('2 of 2 workers')).toBeInTheDocument()
    // Worker rows stay collapsed inside the group until expanded: only the
    // group header's tier badge renders "executor", not the two worker rows.
    expect(screen.getAllByText('executor')).toHaveLength(1)
  })
})
