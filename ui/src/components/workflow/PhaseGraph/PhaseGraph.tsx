import { useMemo, useCallback, useEffect } from 'react'
import { ReactFlow, Background, Controls, useReactFlow, type Node, type Edge, type NodeTypes } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { AgentFlowNode } from './AgentFlowNode'
import { getLayoutedElements } from './layout'
import { useTickingClock } from '@/hooks/useElapsedTime'
import type { PhaseGraphProps, AgentFlowNodeData } from './types'
import type { ActiveAgentV4, AgentSession, AgentHistoryEntry } from '@/types/workflow'

const nodeTypes: NodeTypes = {
  agent: AgentFlowNode,
}

/** Calls fitView() whenever the node set changes (e.g. workflow start, phase transitions). */
function FitViewOnChange({ nodeKey }: { nodeKey: string }) {
  const { fitView } = useReactFlow()
  useEffect(() => {
    // Small delay to let React Flow finish internal layout before fitting
    const timer = setTimeout(() => fitView({ padding: 0.3, duration: 200 }), 50)
    return () => clearTimeout(timer)
  }, [nodeKey, fitView])
  return null
}

export function PhaseGraph({
  phases,
  activeAgents,
  agentHistory,
  phaseOrder,
  sessions,
  onAgentSelect,
}: PhaseGraphProps) {

  // Check if any agents are running
  const hasRunningAgents = useMemo(() => {
    if (!activeAgents) return false
    return Object.values(activeAgents).some(a => !a.result)
  }, [activeAgents])

  // Update elapsed time every second when agents are running
  useTickingClock(hasRunningAgents)

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
    return sessions.find(s =>
      s.agent_type === agent.agent_type &&
      s.phase === phaseName &&
      (!agent.model_id || s.model_id === agent.model_id)
    )
  }, [sessions])

  // Find session for history entry
  const findSessionForHistory = useCallback((entry: AgentHistoryEntry, phaseName: string): AgentSession | undefined => {
    if (!sessions) return undefined

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

    sortedPhaseEntries.forEach(([phaseName, phase], phaseIndex) => {
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
  }, [sortedPhaseEntries, agentsByPhase, agentHistory, handleAgentClick, findSessionForAgent, findSessionForHistory])

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
            type: 'smoothstep',
            animated: isTargetRunning,
            style: { stroke, strokeWidth: 2 },
          })
        })
      })
    }

    return edges
  }, [initialNodes])

  // Apply auto-layout
  const { nodes: layoutedNodes, edges: layoutedEdges } = useMemo(() => {
    return getLayoutedElements(initialNodes, initialEdges, null)
  }, [initialNodes, initialEdges])

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

  // Calculate container height based on number of phase rows
  const phaseIndices = new Set(layoutedNodes.map(n => n.data.phaseIndex))
  const numRows = phaseIndices.size
  const containerHeight = numRows * 150 + 100

  return (
    <div style={{ height: containerHeight }} className="w-full">
      <ReactFlow
        nodes={layoutedNodes}
        edges={layoutedEdges}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.3 }}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        panOnDrag={false}
        zoomOnScroll={false}
        zoomOnPinch={false}
        zoomOnDoubleClick={false}
        minZoom={0.5}
        maxZoom={2}
        preventScrolling={false}
        proOptions={{ hideAttribution: true }}
      >
        <FitViewOnChange nodeKey={nodeKey} />
        <Background color="transparent" />
        <Controls showInteractive={false} position="top-left" />
      </ReactFlow>
    </div>
  )
}
