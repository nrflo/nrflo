import { describe, it, expect, vi } from 'vitest'
import { AgentFlowNode } from './AgentFlowNode'
import { renderWithQuery as render } from '@/test/utils'
import type { AgentFlowNodeData } from './types'
import type { ActiveAgentV4, AgentHistoryEntry } from '@/types/workflow'

// Split out of AgentFlowNode.test.tsx to stay under the file-size ratchet.
vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
}))
vi.mock('@/hooks/useElapsedTime', () => ({ useTickingClock: vi.fn() }))
vi.mock('@/providers/WebSocketProvider', () => ({
  useWebSocketContext: () => ({ addEventListener: vi.fn(), removeEventListener: vi.fn() }),
}))
vi.mock('@/api/stepCursors', () => ({
  fetchStepCursors: vi.fn().mockResolvedValue({ workflow_instance_id: '', cursors: {} }),
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

function makeHistory(overrides: Partial<AgentHistoryEntry> = {}): AgentHistoryEntry {
  return {
    agent_id: 'h1',
    agent_type: 'setup-analyzer',
    model_id: 'claude-sonnet-4-5',
    phase: 'investigation',
    result: 'pass',
    started_at: '2026-01-01T00:00:00Z',
    ended_at: '2026-01-01T00:03:00Z',
    ...overrides,
  }
}

function makeData(overrides: Partial<AgentFlowNodeData> = {}): AgentFlowNodeData {
  return {
    agentKey: 'impl:claude:sonnet',
    phaseName: 'implementation',
    phaseIndex: 0,
    agent: makeAgent(),
    onToggleExpand: vi.fn(),
    ...overrides,
  }
}

describe('AgentFlowNode card sizing', () => {
  // Unified card sizing — all variants use w-[242px] sm:w-[330px] and min-h-[90px]
  it('all card variants use w-[242px] (mobile base) and min-h-[90px]', () => {
    const variants = [
      makeData({ agent: makeAgent({ result: undefined }) }),
      makeData({ agent: undefined, historyEntry: makeHistory({ result: 'pass' }) }),
      makeData({ agent: undefined, historyEntry: makeHistory({ result: 'fail' }) }),
      makeData({ agent: undefined, isPending: true }),
      makeData({ agent: undefined, isSkipped: true }),
      makeData({ agent: undefined, isError: true }),
    ]
    variants.forEach((data) => {
      const { unmount, container } = render(<AgentFlowNode data={data} />)
      const card = container.querySelector('.w-\\[242px\\]')
      expect(card).toBeInTheDocument()
      expect(card?.className).toContain('min-h-[90px]')
      expect(container.querySelector('[class*="min-w"]')).not.toBeInTheDocument()
      unmount()
    })
  })
})
