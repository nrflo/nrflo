import type { PhaseStatus, PhaseState, ActiveAgentV4, AgentHistoryEntry, AgentSession } from '@/types/workflow'

export interface PhaseNodeData {
  name: string
  status: PhaseStatus
  result?: string | null
  startedAt?: string
  endedAt?: string
  error?: string
  isCurrent: boolean
  // Active agents for this phase (can be multiple for same-layer execution)
  activeAgents: ActiveAgentV4[]
  // Agent history entries for this phase
  historyEntries: AgentHistoryEntry[]
}

export interface PhaseEdgeData {
  fromStatus: PhaseStatus
  toStatus: PhaseStatus
  toSkipped: boolean
}

export interface PhaseGraphProps {
  phases: Record<string, PhaseState>
  currentPhase?: string
  activeAgents?: Record<string, ActiveAgentV4>
  agentHistory?: AgentHistoryEntry[]
  // Phase order from config.json (preserved through workflow state)
  phaseOrder?: string[]
  // Agent sessions for displaying messages
  sessions?: AgentSession[]
  // Callback when an agent is clicked (replaces internal modal)
  onAgentSelect?: (data: SelectedAgentData) => void
  // Whether the agent log panel is collapsed — triggers fitView on change
  logPanelCollapsed?: boolean
}

export interface AgentCardProps {
  agent: ActiveAgentV4
  session?: AgentSession
  onExpand?: () => void
  isExpanded?: boolean
}

export interface PhaseNodeProps {
  node: PhaseNodeData
  sessions?: AgentSession[]
  expandedAgentKey?: string | null
  onAgentClick?: (agentKey: string | null) => void
}

// PhaseEdgeProps removed - now using React Flow edges

// Agent-only flow node data for the new graph visualization
export interface AgentFlowNodeData {
  agentKey: string              // Unique key for this agent node
  phaseName: string             // Phase this agent belongs to
  phaseIndex: number            // Order of phase (for edge generation)
  agent?: ActiveAgentV4         // For running agents
  historyEntry?: AgentHistoryEntry  // For completed agents
  session?: AgentSession
  isPending?: boolean           // Phase hasn't started yet
  isSkipped?: boolean           // Phase was skipped
  isCompleted?: boolean         // Phase completed but no history entries
  isError?: boolean             // Phase errored but no history entries
  onToggleExpand: () => void
  [key: string]: unknown        // React Flow compatibility
}

// Data for the agent messages modal
export interface SelectedAgentData {
  phaseName: string
  agent?: ActiveAgentV4
  historyEntry?: AgentHistoryEntry
  session?: AgentSession
}
