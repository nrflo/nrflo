import { describe, it, expect, vi } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { SessionFlowGraph } from './SessionFlowGraph'
import type { SessionFlowResponse } from '@/types/session'

vi.mock('@xyflow/react', async () => {
  const actual = await vi.importActual('@xyflow/react')
  return {
    ...actual,
    ReactFlow: ({ nodes }: { nodes: { id: string }[] }) => (
      <div data-testid="react-flow">
        {nodes.map((n) => (
          <div key={n.id} data-testid={`flow-node-${n.id}`} />
        ))}
      </div>
    ),
    Background: () => <div data-testid="background" />,
  }
})

async function flushLayout() {
  await act(async () => {})
}

function makeFlow(overrides: Partial<SessionFlowResponse> = {}): SessionFlowResponse {
  return {
    root_session_id: 'root',
    nodes: [
      { session_id: 'root', kind: 'delegate', depth: 0, status: 'completed' },
      { session_id: 'child', kind: 'delegate', depth: 1, status: 'completed' },
    ],
    edges: [{ from_session_id: 'root', to_session_id: 'child', kind: 'delegate' }],
    truncated: false,
    ...overrides,
  }
}

describe('SessionFlowGraph', () => {
  it('renders nothing when flow is undefined', async () => {
    const { container } = render(<SessionFlowGraph flow={undefined} />)
    await flushLayout()
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when flow has no nodes', async () => {
    const { container } = render(<SessionFlowGraph flow={makeFlow({ nodes: [], edges: [] })} />)
    await flushLayout()
    expect(container).toBeEmptyDOMElement()
  })

  it('renders one flow node per session-flow node once layout resolves', async () => {
    render(<SessionFlowGraph flow={makeFlow()} />)
    await flushLayout()
    expect(await screen.findByTestId('flow-node-root')).toBeInTheDocument()
    expect(screen.getByTestId('flow-node-child')).toBeInTheDocument()
  })
})
