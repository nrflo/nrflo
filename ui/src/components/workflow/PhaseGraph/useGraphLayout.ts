import { useEffect, useMemo, useRef, useState } from 'react'
import type { Node, Edge } from '@xyflow/react'
import { getLayoutedElements, BASE_HEIGHT, AGENT_NODE_WIDTH, MOBILE_NODE_WIDTH } from './layout'
import type { AgentFlowNodeData } from './types'
import type { CallbackInfo } from '@/types/workflow'

// Async ELK layout with position caching. Layout only depends on the graph
// *structure* (node ids/layers, edge ids, mobile widths) — data-only refreshes
// (WS-driven refetches while agents run) reuse cached positions and skip the
// ELK solve.
export function useGraphLayout(
  initialNodes: Node<AgentFlowNodeData>[],
  initialEdges: Edge[],
  isMobile: boolean,
  callbackInfo?: CallbackInfo,
): { layoutedNodes: Node<AgentFlowNodeData>[]; layoutedEdges: Edge[] } {
  const [layoutedNodes, setLayoutedNodes] = useState<Node<AgentFlowNodeData>[]>([])
  const [layoutedEdges, setLayoutedEdges] = useState<Edge[]>([])
  const layoutCacheRef = useRef<{ key: string; positions: Map<string, { x: number; y: number }> } | null>(null)

  const structureKey = useMemo(
    () =>
      initialNodes.map(n => `${n.id}@${n.data.phaseIndex}`).join(',') +
      '|' + initialEdges.map(e => e.id).join(',') +
      (isMobile ? '|m' : ''),
    [initialNodes, initialEdges, isMobile]
  )

  useEffect(() => {
    const withMergedBadge = (nodes: Node<AgentFlowNodeData>[]): Node<AgentFlowNodeData>[] => {
      if (!(callbackInfo?.requests && callbackInfo.requests.length > 1)) return nodes
      const srcNode = nodes.find(n => n.data.phaseIndex === (callbackInfo.from_layer ?? 0))
      if (!srcNode) return nodes
      return [...nodes, {
        id: 'merged-from-badge',
        type: 'mergedFromBadge',
        position: { x: srcNode.position.x, y: (srcNode.position.y ?? 0) - 30 },
        data: { agentIds: callbackInfo.requests.map(r => r.from_agent) },
      } as unknown as Node<AgentFlowNodeData>]
    }

    const cached = layoutCacheRef.current
    if (cached && cached.key === structureKey) {
      const nodeWidth = isMobile ? MOBILE_NODE_WIDTH : AGENT_NODE_WIDTH
      const reused = initialNodes.map(n => ({
        ...n,
        position: cached.positions.get(n.id) ?? n.position,
        measured: { width: nodeWidth, height: BASE_HEIGHT },
      }))
      setLayoutedNodes(withMergedBadge(reused))
      setLayoutedEdges(initialEdges)
      return
    }

    let cancelled = false
    getLayoutedElements(initialNodes, initialEdges, null, isMobile).then(result => {
      if (!cancelled) {
        layoutCacheRef.current = {
          key: structureKey,
          positions: new Map(result.nodes.map(n => [n.id, { ...n.position }])),
        }
        setLayoutedNodes(withMergedBadge(result.nodes))
        setLayoutedEdges(result.edges)
      }
    })
    return () => { cancelled = true }
  }, [structureKey, initialNodes, initialEdges, isMobile, callbackInfo])

  return { layoutedNodes, layoutedEdges }
}
