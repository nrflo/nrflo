import { useEffect, useState } from 'react'
import { ReactFlow, Background, type Node, type Edge, type NodeTypes } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { SessionFlowNode } from './SessionFlowNode'
import { getSessionFlowLayout, type SessionFlowNodeData } from './sessionFlowLayout'
import type { SessionFlowResponse } from '@/types/session'

const nodeTypes: NodeTypes = {
  sessionFlow: SessionFlowNode as NodeTypes[string],
}

/** Downstream flow canvas for one session: delegations, sub-workflows, dynamic runs, sibling chats. Renders nothing without data. */
export function SessionFlowGraph({ flow }: { flow: SessionFlowResponse | undefined }) {
  const [nodes, setNodes] = useState<Node<SessionFlowNodeData>[]>([])
  const [edges, setEdges] = useState<Edge[]>([])

  useEffect(() => {
    let cancelled = false
    getSessionFlowLayout(flow).then((result) => {
      if (cancelled) return
      setNodes(result.nodes)
      setEdges(result.edges)
    })
    return () => {
      cancelled = true
    }
  }, [flow])

  if (!flow || nodes.length === 0) return null

  return (
    <div className="h-80 w-full border border-border rounded-lg overflow-hidden">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        fitView
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
      </ReactFlow>
    </div>
  )
}
