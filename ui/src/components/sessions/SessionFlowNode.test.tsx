import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { SessionFlowNode } from './SessionFlowNode'
import type { SessionFlowNodeData } from './sessionFlowLayout'
import type { SessionFlowNode as SessionFlowNodeType } from '@/types/session'

vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
}))

function makeNode(overrides: Partial<SessionFlowNodeType> = {}): SessionFlowNodeType {
  return {
    session_id: 'session-abcdef1234',
    kind: 'delegate',
    depth: 1,
    status: 'completed',
    ...overrides,
  }
}

function renderNode(data: Partial<SessionFlowNodeData> = {}) {
  const node = data.node ?? makeNode()
  return render(
    <MemoryRouter>
      <SessionFlowNode data={{ node, isRoot: false, ...data }} />
    </MemoryRouter>
  )
}

describe('SessionFlowNode', () => {
  it('renders truncated session id, kind badge, agent type, and status', () => {
    renderNode({ node: makeNode({ kind: 'delegate', agent_type: 'implementor', status: 'completed' }) })
    expect(screen.getByText('session-')).toBeInTheDocument()
    expect(screen.getByText('delegate')).toBeInTheDocument()
    expect(screen.getByText('implementor')).toBeInTheDocument()
  })

  it('shows a root badge only when isRoot is true', () => {
    const { rerender } = render(
      <MemoryRouter>
        <SessionFlowNode data={{ node: makeNode(), isRoot: true }} />
      </MemoryRouter>
    )
    expect(screen.getByText('root')).toBeInTheDocument()

    rerender(
      <MemoryRouter>
        <SessionFlowNode data={{ node: makeNode(), isRoot: false }} />
      </MemoryRouter>
    )
    expect(screen.queryByText('root')).not.toBeInTheDocument()
  })

  it('shows the result label when present', () => {
    renderNode({ node: makeNode({ result: 'succeeded' }) })
    expect(screen.getByText('succeeded')).toBeInTheDocument()
  })

  it('links a node with a workflow_instance_id to project workflows', () => {
    renderNode({ node: makeNode({ kind: 'subworkflow', workflow_instance_id: 'wfi-1' }) })
    const link = screen.getByRole('link', { name: 'View workflow' })
    expect(link).toHaveAttribute('href', '/project-workflows')
  })

  it('renders no link for a node without a workflow_instance_id', () => {
    renderNode({ node: makeNode({ kind: 'delegate' }) })
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
