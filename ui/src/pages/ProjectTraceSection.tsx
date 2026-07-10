import { ArrowLeft } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/Button'
import { AgentLogPanel } from '@/components/workflow/AgentLogPanel'
import { TraceView } from '@/components/workflow/Trace'
import type { WorkflowState, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from '@/components/workflow/PhaseGraph/types'

interface ProjectTraceSectionProps {
  instanceId: string
  workflowState?: WorkflowState
  sessions: AgentSession[]
  selectedPanelAgent: SelectedAgentData | null
  onAgentSelect: (data: SelectedAgentData | null) => void
  logPanelCollapsed: boolean
  onClose: () => void
  onResumeSession: (sessionId: string) => void
  resumePending: boolean
  projectFindings?: Record<string, unknown>
}

/** Full-width trace timeline for one project-scoped run, with back navigation. */
export function ProjectTraceSection({
  instanceId,
  workflowState,
  sessions,
  selectedPanelAgent,
  onAgentSelect,
  logPanelCollapsed,
  onClose,
  onResumeSession,
  resumePending,
  projectFindings,
}: ProjectTraceSectionProps) {
  return (
    <div className="space-y-3">
      <Button variant="outline" size="sm" onClick={onClose}>
        <ArrowLeft className="h-3.5 w-3.5 mr-1" />
        Back
      </Button>
      <div className={cn('flex flex-col md:flex-row gap-0', selectedPanelAgent && 'min-h-[calc(100vh-280px)]')}>
        <div className="flex-1 min-w-0">
          <TraceView
            instanceId={instanceId}
            sessions={sessions}
            workflowState={workflowState}
            onAgentSelect={onAgentSelect}
          />
        </div>
        {selectedPanelAgent && (
          <AgentLogPanel
            activeAgents={workflowState?.active_agents ?? {}}
            sessions={sessions}
            collapsed={logPanelCollapsed}
            selectedAgent={selectedPanelAgent}
            onAgentSelect={onAgentSelect}
            onResumeSession={onResumeSession}
            resumePending={resumePending}
            agentFindings={workflowState?.findings}
            projectFindings={projectFindings}
            phaseLayers={workflowState?.phase_layers}
            workflowFindings={workflowState?.workflow_findings}
          />
        )}
      </div>
    </div>
  )
}
