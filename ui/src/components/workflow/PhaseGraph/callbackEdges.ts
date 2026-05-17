import { MarkerType } from '@xyflow/react'
import type { Edge, Node } from '@xyflow/react'
import type { AgentFlowNodeData } from './types'
import type { CallbackInfo, CallbackPlanStep } from '@/types/workflow'

export function buildCallbackSteps(callbackInfo: CallbackInfo): CallbackPlanStep[] {
  if (callbackInfo.plan && callbackInfo.plan.length > 0) {
    return [...callbackInfo.plan].sort((a, b) => a.layer - b.layer)
  }
  const layer = callbackInfo.level ?? callbackInfo.to_layer ?? 0
  return [{ layer, mode: 'whole_layer', instructions: callbackInfo.instructions ?? '' }]
}

export function findCallbackSourceNode(
  nodesByPhase: Record<number, Node<AgentFlowNodeData>[]>,
  fromLayer: number,
  fromAgent?: string,
): Node<AgentFlowNodeData> | undefined {
  let fallback: Node<AgentFlowNodeData> | undefined
  for (const nodes of Object.values(nodesByPhase)) {
    for (const node of nodes) {
      if (node.data.phaseIndex === fromLayer) {
        const agentType = node.data.agent?.agent_type || node.data.historyEntry?.agent_type
        if (agentType === fromAgent) return node
        if (!fallback) fallback = node
      }
    }
  }
  return fallback
}

function truncateInstructions(text: string): string {
  return text.length > 60 ? text.slice(0, 57) + '...' : text
}

const EDGE_STYLE = { stroke: '#3b82f6', strokeWidth: 2, strokeDasharray: '6 4' } as const
const EDGE_MARKER = { type: MarkerType.ArrowClosed, color: '#3b82f6', width: 20, height: 20 } as const

function makeEdge(id: string, source: string, target: string, label: string): Edge {
  return {
    id,
    source,
    target,
    sourceHandle: 'callback-source',
    targetHandle: 'callback-target',
    type: 'smoothstep',
    animated: true,
    style: EDGE_STYLE,
    markerEnd: EDGE_MARKER,
    label,
    labelStyle: { fill: '#3b82f6', fontSize: 11, fontWeight: 500 },
    labelBgStyle: { fill: '#eff6ff', stroke: '#3b82f6', strokeWidth: 0.5 },
    labelBgPadding: [6, 4] as [number, number],
    labelBgBorderRadius: 4,
  }
}

export function buildCallbackEdges(
  callbackInfo: CallbackInfo,
  nodesByPhase: Record<number, Node<AgentFlowNodeData>[]>,
): Edge[] {
  const steps = buildCallbackSteps(callbackInfo)
  const sourceNode = findCallbackSourceNode(
    nodesByPhase,
    callbackInfo.from_layer ?? 0,
    callbackInfo.from_agent,
  )
  if (!sourceNode) return []

  const totalEdgeCount = steps.reduce((acc, step) => {
    if (step.mode === 'agents' && step.agents) return acc + step.agents.length
    return acc + 1
  }, 0)
  const isSingle = totalEdgeCount === 1
  const edges: Edge[] = []

  steps.forEach((step, stepIdx) => {
    const baseInstructions = step.instructions ?? callbackInfo.instructions ?? ''

    if (step.mode === 'agents' && step.agents && step.agents.length > 0) {
      step.agents.forEach((agentId, agentIdx) => {
        let targetNode: Node<AgentFlowNodeData> | undefined
        for (const nodes of Object.values(nodesByPhase)) {
          for (const node of nodes) {
            if (node.data.phaseIndex === step.layer) {
              const agentType = node.data.agent?.agent_type || node.data.historyEntry?.agent_type
              if (agentType === agentId) { targetNode = node; break }
            }
          }
          if (targetNode) break
        }
        if (!targetNode) return
        const edgeId = isSingle ? 'callback-edge' : `callback-edge-${stepIdx}-${agentIdx}`
        const label = isSingle
          ? truncateInstructions(baseInstructions)
          : `[agents] ${truncateInstructions(baseInstructions)}`
        edges.push(makeEdge(edgeId, sourceNode.id, targetNode.id, label))
      })
    } else {
      let targetNode: Node<AgentFlowNodeData> | undefined
      for (const nodes of Object.values(nodesByPhase)) {
        for (const node of nodes) {
          if (node.data.phaseIndex === step.layer) targetNode = node
        }
      }
      if (!targetNode) return
      const edgeId = isSingle ? 'callback-edge' : `callback-edge-${stepIdx}-0`
      const label = isSingle
        ? truncateInstructions(baseInstructions)
        : `[layer] ${truncateInstructions(baseInstructions)}`
      edges.push(makeEdge(edgeId, sourceNode.id, targetNode.id, label))
    }
  })

  return edges
}
