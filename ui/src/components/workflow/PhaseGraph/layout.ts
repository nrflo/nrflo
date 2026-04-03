import ELK from 'elkjs/lib/elk.bundled.js'
import type { Node, Edge } from '@xyflow/react'
import type { AgentFlowNodeData } from './types'

export const AGENT_NODE_WIDTH = 352
export const MOBILE_NODE_WIDTH = 264
export const BASE_HEIGHT = 110
export const EXPANDED_HEIGHT = 420  // Base + messages area

const elk = new ELK()

/** Build synthetic edges between adjacent layers so ELK places same-layer nodes in the same row. */
function buildLayerEdges(nodes: Node<AgentFlowNodeData>[], existingEdges: Edge[]) {
  const existingSources = new Set(existingEdges.map(e => e.source))
  const existingTargets = new Set(existingEdges.map(e => e.target))

  // Group node IDs by phaseIndex
  const byPhase: Record<number, string[]> = {}
  for (const node of nodes) {
    const idx = node.data.phaseIndex
    if (!byPhase[idx]) byPhase[idx] = []
    byPhase[idx].push(node.id)
  }

  const phases = Object.keys(byPhase).map(Number).sort((a, b) => a - b)
  const syntheticEdges: { id: string; sources: string[]; targets: string[] }[] = []

  for (let i = 0; i < phases.length - 1; i++) {
    const currentIds = byPhase[phases[i]]
    const nextIds = byPhase[phases[i + 1]]
    for (const src of currentIds) {
      for (const tgt of nextIds) {
        // Only add if no existing edge already connects these
        if (!(existingSources.has(src) && existingTargets.has(tgt))) {
          syntheticEdges.push({
            id: `_synth_${src}_${tgt}`,
            sources: [src],
            targets: [tgt],
          })
        }
      }
    }
  }

  return syntheticEdges
}

/** Center each layer's nodes around the same horizontal midpoint. */
function centerLayers(nodes: Node<AgentFlowNodeData>[], nodeWidth: number) {
  // Group nodes by phaseIndex (layer)
  const byLayer: Record<number, Node<AgentFlowNodeData>[]> = {}
  for (const node of nodes) {
    const layer = node.data.phaseIndex
    if (!byLayer[layer]) byLayer[layer] = []
    byLayer[layer].push(node)
  }

  // Find the widest layer's extent to use as the center reference
  let globalCenter = 0
  let maxExtent = 0
  for (const layerNodes of Object.values(byLayer)) {
    const minX = Math.min(...layerNodes.map(n => n.position.x))
    const maxX = Math.max(...layerNodes.map(n => n.position.x)) + nodeWidth
    const extent = maxX - minX
    if (extent > maxExtent) {
      maxExtent = extent
      globalCenter = (minX + maxX) / 2
    }
  }

  // Shift each layer so its center aligns with globalCenter
  for (const layerNodes of Object.values(byLayer)) {
    const minX = Math.min(...layerNodes.map(n => n.position.x))
    const maxX = Math.max(...layerNodes.map(n => n.position.x)) + nodeWidth
    const layerCenter = (minX + maxX) / 2
    const shift = globalCenter - layerCenter
    if (Math.abs(shift) > 0.5) {
      for (const node of layerNodes) {
        node.position.x += shift
      }
    }
  }
}

export async function getLayoutedElements(
  nodes: Node<AgentFlowNodeData>[],
  edges: Edge[],
  expandedAgentKey: string | null,
  isMobile = false
): Promise<{ nodes: Node<AgentFlowNodeData>[]; edges: Edge[] }> {
  if (nodes.length === 0) return { nodes, edges }

  const nodeWidth = isMobile ? MOBILE_NODE_WIDTH : AGENT_NODE_WIDTH
  const betweenLayersSpacing = isMobile ? '60' : '120'
  const nodeSpacing = isMobile ? '30' : '60'

  const elkEdges = edges.map(edge => ({
    id: edge.id,
    sources: [edge.source],
    targets: [edge.target],
  }))

  // Add synthetic edges for layer connectivity so ELK assigns correct layers
  const syntheticEdges = buildLayerEdges(nodes, edges)

  const elkGraph = {
    id: 'root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'DOWN',
      'elk.layered.spacing.nodeNodeBetweenLayers': betweenLayersSpacing,
      'elk.spacing.nodeNode': nodeSpacing,
      'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX',
      'elk.alignment': 'CENTER',
      'elk.partitioning.activate': 'true',
    },
    children: nodes.map(node => {
      const isExpanded = expandedAgentKey === node.id
      const height = isExpanded ? EXPANDED_HEIGHT : BASE_HEIGHT
      return {
        id: node.id,
        width: nodeWidth,
        height,
        layoutOptions: {
          'elk.partitioning.partition': String(node.data.phaseIndex),
        },
      }
    }),
    edges: [...elkEdges, ...syntheticEdges],
  }

  const layout = await elk.layout(elkGraph)

  const nodeMap = new Map(
    (layout.children ?? []).map(child => [child.id, child])
  )

  // Apply ELK positions
  for (const node of nodes) {
    const elkNode = nodeMap.get(node.id)
    if (elkNode) {
      node.position = { x: elkNode.x ?? 0, y: elkNode.y ?? 0 }
      const isExpanded = expandedAgentKey === node.id
      node.measured = {
        width: nodeWidth,
        height: isExpanded ? EXPANDED_HEIGHT : BASE_HEIGHT,
      }
    }
  }

  // Normalize Y so topmost node starts at 0
  const minY = Math.min(...nodes.map(n => n.position.y))
  if (minY > 0) {
    for (const node of nodes) {
      node.position.y -= minY
    }
  }

  // Center each layer horizontally around the widest layer's midpoint.
  // ELK's NETWORK_SIMPLEX doesn't center 1→2→1 fan patterns well.
  centerLayers(nodes, nodeWidth)

  return { nodes, edges }
}
