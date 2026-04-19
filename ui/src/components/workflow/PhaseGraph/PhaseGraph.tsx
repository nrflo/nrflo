import { useMemo, useState, useCallback, useEffect, useRef } from 'react'
import { ReactFlow, Background, useReactFlow, useStore, MarkerType, type Node, type Edge, type NodeTypes } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { AgentFlowNode } from './AgentFlowNode'
import { getLayoutedElements, BASE_HEIGHT } from './layout'
import { useIsMobile } from '@/hooks/useIsMobile'
import { AutoCenterInterval, PhaseGraphControls } from './PhaseGraphControls'
import { FIT_VIEW_OPTIONS, performFitView } from './fitViewOptions'
import type { PhaseGraphProps, AgentFlowNodeData } from './types'
import type { ActiveAgentV4, AgentSession, AgentHistoryEntry } from '@/types/workflow'

const nodeTypes: NodeTypes = {
  agent: AgentFlowNode,
}

// 4 vertical ControlButtons at 28px each + 8px top offset + safety padding.
const CONTROLS_MIN_HEIGHT = 140

/** Calls fitView() when container dimensions change (via ResizeObserver) or node layout changes. */
function FitViewOnChange({ nodeKey }: { nodeKey: string }) {
  const { fitView } = useReactFlow()
  const containerWidth = useStore((s) => s.width)
  const containerHeight = useStore((s) => s.height)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => performFitView(fitView), 150)
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current) }
  }, [containerWidth, containerHeight, fitView])

  useEffect(() => {
    const timer = setTimeout(() => performFitView(fitView), 100)
    return () => clearTimeout(timer)
  }, [nodeKey, fitView])

  return null
}

export function PhaseGraph({
  phases,
  activeAgents,
  agentHistory,
  phaseOrder,
  phaseLayers,
  sessions,
  onAgentSelect,
  onRetryFailed,
  retryingSessionId,
  workflowStatus,
  callbackInfo,
}: PhaseGraphProps) {

  const isMobile = useIsMobile()
  const [autoCenter, setAutoCenter] = useState(true)

  // Build phase start times map from agent history
  const phaseStartTimes: Record<string, number> = useMemo(() => {
    const times: Record<string, number> = {}
    if (agentHistory) {
      for (const entry of agentHistory) {
        if (entry.started_at && entry.phase) {
          const time = new Date(entry.started_at).getTime()
          if (!times[entry.phase] || time < times[entry.phase]) {
            times[entry.phase] = time
          }
        }
      }
    }
    return times
  }, [agentHistory])

  // Build sorted phase entries from phase_order (shows ALL phases from config)
  // Falls back to phases object if phase_order not available
  const sortedPhaseEntries = useMemo(() => {
    // If we have phase_order from backend, use it as source of truth for ALL phases
    if (phaseOrder && phaseOrder.length > 0) {
      return phaseOrder.map(phaseName => {
        const phase = phases[phaseName] || { status: 'pending' }
        return [phaseName, phase] as const
      })
    }

    // Fallback to phases object entries sorted by start time
    const entries = Object.entries(phases)
    return entries.sort(([nameA], [nameB]) => {
      const timeA = phaseStartTimes[nameA]
      const timeB = phaseStartTimes[nameB]
      if (!timeA && !timeB) return 0
      if (!timeA) return 1
      if (!timeB) return -1
      return timeA - timeB
    })
  }, [phases, phaseOrder, phaseStartTimes])

  // Group active agents by phase (use phase field, fallback to agent_type)
  const agentsByPhase = useMemo(() => {
    const byPhase: Record<string, ActiveAgentV4[]> = {}
    if (activeAgents) {
      for (const [, agent] of Object.entries(activeAgents)) {
        const phaseName = agent.phase || agent.agent_type
        if (!byPhase[phaseName]) {
          byPhase[phaseName] = []
        }
        byPhase[phaseName].push(agent)
      }
    }
    return byPhase
  }, [activeAgents])

  // Memoized callback for agent click - notifies parent via onAgentSelect
  const handleAgentClick = useCallback((data: { phaseName: string; agent?: ActiveAgentV4; historyEntry?: AgentHistoryEntry; session?: AgentSession }) => {
    onAgentSelect?.(data)
  }, [onAgentSelect])

  // Find session for running agent
  const findSessionForAgent = useCallback((agent: ActiveAgentV4, phaseName: string): AgentSession | undefined => {
    if (!sessions) return undefined
    if (agent.session_id) {
      const byId = sessions.find(s => s.id === agent.session_id)
      if (byId) return byId
    }
    return sessions.find(s =>
      s.agent_type === agent.agent_type &&
      s.phase === phaseName &&
      (!agent.model_id || s.model_id === agent.model_id)
    )
  }, [sessions])

  // Find session for history entry
  const findSessionForHistory = useCallback((entry: AgentHistoryEntry, phaseName: string): AgentSession | undefined => {
    if (!sessions) return undefined

    // Prefer exact session_id match
    if (entry.session_id) {
      const byId = sessions.find(s => s.id === entry.session_id)
      if (byId) return byId
    }

    // First try exact match with model_id
    const exactMatch = sessions.find(s =>
      s.agent_type === entry.agent_type &&
      s.phase === phaseName &&
      s.model_id === entry.model_id &&
      s.status !== 'running'
    )
    if (exactMatch) return exactMatch

    // Fallback: match by agent_type and phase only
    return sessions.find(s =>
      s.agent_type === entry.agent_type &&
      s.phase === phaseName &&
      s.status !== 'running'
    )
  }, [sessions])

  // Build React Flow nodes - AGENT ONLY (no phase nodes)
  const initialNodes: Node<AgentFlowNodeData>[] = useMemo(() => {
    const nodes: Node<AgentFlowNodeData>[] = []

    sortedPhaseEntries.forEach(([phaseName, phase], idx) => {
      const phaseIndex = phaseLayers?.[phaseName] ?? idx
      const phaseAgents = agentsByPhase[phaseName] || []
      const history = agentHistory?.filter(h => h.phase === phaseName) || []

      // Running agents (phase is in_progress)
      if (phase.status === 'in_progress' && phaseAgents.length > 0) {
        phaseAgents.forEach((agent, i) => {
          const agentKey = `${phaseName}-${i}`
          const session = findSessionForAgent(agent, phaseName)
          nodes.push({
            id: agentKey,
            type: 'agent',
            position: { x: 0, y: 0 },
            data: {
              agentKey,
              phaseName,
              phaseIndex,
              agent,
              session,
              onToggleExpand: () => handleAgentClick({ phaseName, agent, session }),
            }
          })
        })
      }
      // Completed agents from history
      else if ((phase.status === 'completed' || phase.status === 'error') && history.length > 0) {
        history.forEach((entry, i) => {
          const agentKey = `${phaseName}-history-${i}`
          const session = findSessionForHistory(entry, phaseName)
          nodes.push({
            id: agentKey,
            type: 'agent',
            position: { x: 0, y: 0 },
            data: {
              agentKey,
              phaseName,
              phaseIndex,
              historyEntry: entry,
              session,
              onToggleExpand: () => handleAgentClick({ phaseName, historyEntry: entry, session }),
              onRetryFailed,
              retryingSessionId,
              workflowStatus,
            }
          })
        })
      }
      // Completed/error phase with no history entries - show completed placeholder
      else if ((phase.status === 'completed' || phase.status === 'error') && history.length === 0) {
        const agentKey = `${phaseName}-completed`
        nodes.push({
          id: agentKey,
          type: 'agent',
          position: { x: 0, y: 0 },
          data: {
            agentKey,
            phaseName,
            phaseIndex,
            isPending: false,
            isCompleted: true,
            isError: phase.status === 'error',
            onToggleExpand: () => {},
          }
        })
      }
      // Pending or skipped phases - show placeholder node
      else {
        const agentKey = `${phaseName}-pending`
        nodes.push({
          id: agentKey,
          type: 'agent',
          position: { x: 0, y: 0 },
          data: {
            agentKey,
            phaseName,
            phaseIndex,
            isPending: true,
            isSkipped: phase.status === 'skipped',
            onToggleExpand: () => {}, // No modal for pending phases
          }
        })
      }
    })

    return nodes
  }, [sortedPhaseEntries, agentsByPhase, agentHistory, phaseLayers, handleAgentClick, findSessionForAgent, findSessionForHistory, onRetryFailed, retryingSessionId, workflowStatus])

  // Build React Flow edges with layer-based branching pattern
  const initialEdges: Edge[] = useMemo(() => {
    const edges: Edge[] = []

    // Group nodes by phaseIndex
    const nodesByPhase: Record<number, Node<AgentFlowNodeData>[]> = {}
    initialNodes.forEach(node => {
      const idx = node.data.phaseIndex
      if (!nodesByPhase[idx]) nodesByPhase[idx] = []
      nodesByPhase[idx].push(node)
    })

    // Connect each phase's agents to next phase's agents
    const phaseIndices = Object.keys(nodesByPhase).map(Number).sort((a, b) => a - b)

    for (let i = 0; i < phaseIndices.length - 1; i++) {
      const currentNodes = nodesByPhase[phaseIndices[i]]
      const nextNodes = nodesByPhase[phaseIndices[i + 1]]

      if (!currentNodes || !nextNodes) continue

      // Connect all current → all next (branching/merging)
      currentNodes.forEach(fromNode => {
        nextNodes.forEach(toNode => {
          // Determine if edge should be animated (target is running)
          const isTargetRunning = toNode.data.agent && !toNode.data.agent.result
          // Determine edge color based on source result
          const sourceResult = fromNode.data.agent?.result || fromNode.data.historyEntry?.result
          let stroke = '#d1d5db' // gray-300 default
          if (sourceResult === 'pass') {
            stroke = '#22c55e' // green-500
          } else if (sourceResult === 'fail') {
            stroke = '#ef4444' // red-500
          } else if (fromNode.data.agent && !fromNode.data.agent.result) {
            stroke = '#facc15' // yellow-400 for running
          }

          edges.push({
            id: `${fromNode.id}-${toNode.id}`,
            source: fromNode.id,
            target: toNode.id,
            type: 'default',
            animated: isTargetRunning,
            style: { stroke, strokeWidth: 2 },
            markerEnd: { type: MarkerType.ArrowClosed, color: stroke, width: 20, height: 20 },
          })
        })
      })
    }

    // Callback edge: blue animated arrow from callback source to target layer
    if (callbackInfo) {
      // Find source node: match from_agent at from_layer
      let sourceNode: Node<AgentFlowNodeData> | undefined
      for (const nodes of Object.values(nodesByPhase)) {
        for (const node of nodes) {
          if (node.data.phaseIndex === callbackInfo.from_layer) {
            const agentType = node.data.agent?.agent_type || node.data.historyEntry?.agent_type
            if (agentType === callbackInfo.from_agent) {
              sourceNode = node
              break
            }
            // Track as fallback (any node at from_layer)
            if (!sourceNode) sourceNode = node
          }
        }
        if (sourceNode) {
          const agentType = sourceNode.data.agent?.agent_type || sourceNode.data.historyEntry?.agent_type
          if (agentType === callbackInfo.from_agent) break
        }
      }

      // Find target node: last node (rightmost proxy) at target layer
      let targetNode: Node<AgentFlowNodeData> | undefined
      for (const nodes of Object.values(nodesByPhase)) {
        for (const node of nodes) {
          if (node.data.phaseIndex === callbackInfo.level) {
            targetNode = node // last one wins = rightmost proxy
          }
        }
      }

      if (sourceNode && targetNode) {
        const label = callbackInfo.instructions.length > 60
          ? callbackInfo.instructions.slice(0, 57) + '...'
          : callbackInfo.instructions
        edges.push({
          id: 'callback-edge',
          source: sourceNode.id,
          target: targetNode.id,
          sourceHandle: 'callback-source',
          targetHandle: 'callback-target',
          type: 'smoothstep',
          animated: true,
          style: { stroke: '#3b82f6', strokeWidth: 2, strokeDasharray: '6 4' },
          markerEnd: { type: MarkerType.ArrowClosed, color: '#3b82f6', width: 20, height: 20 },
          label,
          labelStyle: { fill: '#3b82f6', fontSize: 11, fontWeight: 500 },
          labelBgStyle: { fill: '#eff6ff', stroke: '#3b82f6', strokeWidth: 0.5 },
          labelBgPadding: [6, 4] as [number, number],
          labelBgBorderRadius: 4,
        })
      }
    }

    return edges
  }, [initialNodes, callbackInfo])

  // Apply async ELK layout
  const [layoutedNodes, setLayoutedNodes] = useState<Node<AgentFlowNodeData>[]>([])
  const [layoutedEdges, setLayoutedEdges] = useState<Edge[]>([])

  useEffect(() => {
    let cancelled = false
    getLayoutedElements(initialNodes, initialEdges, null, isMobile).then(result => {
      if (!cancelled) {
        setLayoutedNodes(result.nodes)
        setLayoutedEdges(result.edges)
      }
    })
    return () => { cancelled = true }
  }, [initialNodes, initialEdges, isMobile])

  // Stable key derived from node IDs to trigger fitView on node set changes
  const nodeKey = useMemo(
    () => layoutedNodes.map(n => n.id).join(','),
    [layoutedNodes]
  )

  if (layoutedNodes.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        No workflow phases defined
      </p>
    )
  }

  // Calculate container height from actual layout output
  const containerHeight = Math.max(
    ...layoutedNodes.map(n => (n.position.y || 0) + (n.measured?.height || BASE_HEIGHT))
  ) + 50

  return (
    <div style={{ height: Math.max(containerHeight, CONTROLS_MIN_HEIGHT) }} className="w-full">
      <ReactFlow
        nodes={layoutedNodes}
        edges={layoutedEdges}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={FIT_VIEW_OPTIONS}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        panOnDrag={isMobile}
        zoomOnScroll={false}
        zoomOnPinch={true}
        zoomOnDoubleClick={false}
        minZoom={0.3}
        maxZoom={2}
        preventScrolling={false}
        proOptions={{ hideAttribution: true }}
      >
        <FitViewOnChange nodeKey={nodeKey} />
        <AutoCenterInterval enabled={autoCenter} />
        <Background color="transparent" />
        <PhaseGraphControls autoCenter={autoCenter} setAutoCenter={setAutoCenter} />
      </ReactFlow>
    </div>
  )
}
