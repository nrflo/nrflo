import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SessionDetail } from './SessionDetail'
import { useSessionFlow, useSessionStats } from '@/hooks/useSessionFlow'

vi.mock('@/hooks/useSessionFlow')
vi.mock('./SessionFlowGraph', () => ({
  SessionFlowGraph: () => <div>flow-graph</div>,
}))
vi.mock('./SessionToolDistribution', () => ({
  SessionToolDistribution: () => <div>tool-distribution</div>,
}))
vi.mock('./SessionCostRollup', () => ({
  SessionCostRollup: () => <div>cost-rollup</div>,
}))

function mockQueries({
  flow,
  flowLoading = false,
  stats,
  statsLoading = false,
}: {
  flow?: { nodes: unknown[] }
  flowLoading?: boolean
  stats?: { tool_calls?: unknown[]; subtree_cost_usd?: number; subtree_tokens?: number }
  statsLoading?: boolean
} = {}) {
  vi.mocked(useSessionFlow).mockReturnValue({ data: flow, isLoading: flowLoading } as any)
  vi.mocked(useSessionStats).mockReturnValue({ data: stats, isLoading: statsLoading } as any)
}

describe('SessionDetail', () => {
  it('shows a loading state while flow or stats are pending', () => {
    mockQueries({ flowLoading: true })
    render(<SessionDetail sessionId="session-abcdef1234" />)
    expect(screen.getByText('Loading...')).toBeInTheDocument()
    expect(screen.queryByText('flow-graph')).not.toBeInTheDocument()
  })

  it('shows an empty state when there is no flow, no tool calls, and no cost rollup', () => {
    mockQueries({ flow: { nodes: [] }, stats: { tool_calls: [] } })
    render(<SessionDetail sessionId="session-abcdef1234" />)
    expect(screen.getByText('No downstream activity recorded for this session.')).toBeInTheDocument()
    expect(screen.queryByText('flow-graph')).not.toBeInTheDocument()
  })

  it('renders the flow graph, tool distribution, and cost rollup once data resolves', () => {
    mockQueries({
      flow: { nodes: [{ id: 'n1' }] },
      stats: { tool_calls: [{ tool_name: 'Read' }] },
    })
    render(<SessionDetail sessionId="session-abcdef1234" />)
    expect(screen.getByText('flow-graph')).toBeInTheDocument()
    expect(screen.getByText('tool-distribution')).toBeInTheDocument()
    expect(screen.getByText('cost-rollup')).toBeInTheDocument()
    expect(screen.queryByText('No downstream activity recorded for this session.')).not.toBeInTheDocument()
  })

  it('renders the truncated session id heading', () => {
    mockQueries({ flow: { nodes: [] }, stats: {} })
    render(<SessionDetail sessionId="session-abcdef1234" />)
    expect(screen.getByText('Session session-')).toBeInTheDocument()
  })
})
