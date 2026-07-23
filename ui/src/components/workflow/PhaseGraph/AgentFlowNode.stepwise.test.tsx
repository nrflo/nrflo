import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { AgentFlowNode } from './AgentFlowNode'
import { renderWithQuery } from '@/test/utils'
import { useStepCursors } from '@/hooks/useStepCursors'
import type { AgentFlowNodeData } from './types'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'
import type { StepCursorProgress, StepCursorsResponse } from '@/types/stepwise'

vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
}))

vi.mock('@/hooks/useElapsedTime', () => ({
  useTickingClock: vi.fn(),
}))

vi.mock('@/hooks/useStepCursors')

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
    restart_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:03:00Z',
    ...overrides,
  }
}

function makeCursor(overrides: Partial<StepCursorProgress> = {}): StepCursorProgress {
  return {
    node_id: 'implementation',
    revision: 1,
    current_index: 1,
    total: 3,
    done: false,
    updated_at: '2026-01-01T00:00:00Z',
    steps: [
      { step_id: 's1', title: 'Write tests', status: 'done' },
      { step_id: 's2', title: 'Implement', status: 'active' },
      { step_id: 's3', title: 'Verify', status: 'pending' },
    ],
    ...overrides,
  }
}

function makeData(overrides: Partial<AgentFlowNodeData> = {}): AgentFlowNodeData {
  return {
    agentKey: 'impl:claude:sonnet',
    phaseName: 'implementation',
    phaseIndex: 0,
    agent: makeAgent(),
    session: makeSession(),
    onToggleExpand: vi.fn(),
    ...overrides,
  }
}

function mockCursors(cursors: Record<string, StepCursorProgress>) {
  // Mirrors useStepCursors' real `enabled: !!instanceId` gating: no
  // instanceId means no data, same as a disabled query.
  vi.mocked(useStepCursors).mockImplementation((instanceId?: string) =>
    ({
      data: instanceId ? ({ workflow_instance_id: instanceId, cursors } as StepCursorsResponse) : undefined,
    }) as ReturnType<typeof useStepCursors>
  )
}

describe('AgentFlowNode stepwise progress', () => {
  it('renders the step progress strip for a running stepwise agent whose session/phaseName resolve to a cursor', () => {
    mockCursors({ implementation: makeCursor() })
    renderWithQuery(<AgentFlowNode data={makeData()} />)

    expect(screen.getByText('2/3')).toBeInTheDocument()
  })

  it('renders exactly as before (no strip) when the instance has no cursors', () => {
    mockCursors({})
    renderWithQuery(<AgentFlowNode data={makeData()} />)

    expect(screen.queryByText(/^\d+\/\d+$/)).not.toBeInTheDocument()
  })

  it('renders no strip for a non-stepwise agent with no session at all', () => {
    mockCursors({ implementation: makeCursor() })
    renderWithQuery(<AgentFlowNode data={makeData({ session: undefined })} />)

    expect(screen.queryByText(/^\d+\/\d+$/)).not.toBeInTheDocument()
  })
})
