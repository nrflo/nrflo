import type { Node, Edge } from '@xyflow/react'
import type { AgentFlowNodeData } from './types'

const AGENT_NODE_WIDTH = 320
const HORIZONTAL_GAP = 60
const VERTICAL_GAP = 120
const BASE_HEIGHT = 110
const EXPANDED_HEIGHT = 420  // Base + messages area

export function getLayoutedElements(
  nodes: Node<AgentFlowNodeData>[],
  edges: Edge[],
  expandedAgentKey: string | null
): { nodes: Node<AgentFlowNodeData>[]; edges: Edge[] } {
  // Group nodes by phaseIndex
  const nodesByPhase: Record<number, Node<AgentFlowNodeData>[]> = {}
  nodes.forEach(node => {
    const idx = node.data.phaseIndex
    if (!nodesByPhase[idx]) nodesByPhase[idx] = []
    nodesByPhase[idx].push(node)
  })

  let currentY = 0
  const phaseIndices = Object.keys(nodesByPhase).map(Number).sort((a, b) => a - b)

  phaseIndices.forEach(phaseIdx => {
    const phaseNodes = nodesByPhase[phaseIdx]
    const count = phaseNodes.length

    // Calculate max height for this row (if any expanded)
    let maxHeight = BASE_HEIGHT
    phaseNodes.forEach(node => {
      if (expandedAgentKey === node.id) {
        maxHeight = EXPANDED_HEIGHT
      }
    })

    // Center agents horizontally
    const totalWidth = count * AGENT_NODE_WIDTH + (count - 1) * HORIZONTAL_GAP
    const startX = -totalWidth / 2

    phaseNodes.forEach((node, i) => {
      const isExpanded = expandedAgentKey === node.id
      node.position = {
        x: startX + i * (AGENT_NODE_WIDTH + HORIZONTAL_GAP),
        y: currentY
      }
      node.measured = {
        width: AGENT_NODE_WIDTH,
        height: isExpanded ? EXPANDED_HEIGHT : BASE_HEIGHT
      }
    })

    currentY += maxHeight + VERTICAL_GAP
  })

  return { nodes, edges }
}
