import type { WorkflowFindings } from '@/types/workflow'
import { FindingRow, isInternalKey } from './FindingsPanel'

interface AllFindingsPanelProps {
  workflowFindings?: Record<string, unknown>
  agentFindings?: WorkflowFindings
  projectFindings?: Record<string, unknown>
  phaseLayers?: Record<string, number>
}

function getLayerForAgent(agentKey: string, phaseLayers?: Record<string, number>): number | undefined {
  if (!phaseLayers) return undefined
  if (agentKey in phaseLayers) return phaseLayers[agentKey]
  // Strip model suffix (e.g., "implementor:claude-sonnet-4-5" -> "implementor")
  const base = agentKey.split(':')[0]
  if (base in phaseLayers) return phaseLayers[base]
  return undefined
}

export function AllFindingsPanel({ workflowFindings, agentFindings, projectFindings, phaseLayers }: AllFindingsPanelProps) {
  // Workflow findings (non-internal)
  const wfKeys = workflowFindings ? Object.keys(workflowFindings).filter(k => !isInternalKey(k)) : []

  // Project findings (non-internal)
  const projKeys = projectFindings ? Object.keys(projectFindings).filter(k => !isInternalKey(k)) : []

  // Agent findings sorted by layer
  const agentEntries = agentFindings
    ? Object.entries(agentFindings).filter(([key]) => !isInternalKey(key))
    : []
  const sortedAgents = agentEntries
    .map(([key, findings]) => ({
      key,
      findings,
      layer: getLayerForAgent(key, phaseLayers),
    }))
    .filter(({ findings }) => typeof findings === 'object' && findings !== null && Object.keys(findings).filter(k => !isInternalKey(k)).length > 0)
    .sort((a, b) => {
      if (a.layer === undefined && b.layer === undefined) return 0
      if (a.layer === undefined) return 1
      if (b.layer === undefined) return -1
      return a.layer - b.layer
    })

  const hasWorkflow = wfKeys.length > 0
  const hasProject = projKeys.length > 0
  const hasAgents = sortedAgents.length > 0

  if (!hasWorkflow && !hasProject && !hasAgents) {
    return (
      <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
        <p className="text-xs">No findings available</p>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {hasWorkflow && (
        <div>
          <h4 className="text-sm font-medium text-foreground px-2 mb-1">Workflow Findings</h4>
          <div className="border border-border rounded">
            {wfKeys.map(key => (
              <FindingRow key={key} findingKey={key} value={workflowFindings![key]} />
            ))}
          </div>
        </div>
      )}
      {hasProject && (
        <div>
          <h4 className="text-sm font-medium text-foreground px-2 mb-1">Project Findings</h4>
          <div className="border border-border rounded">
            {projKeys.map(key => (
              <FindingRow key={key} findingKey={key} value={projectFindings![key]} />
            ))}
          </div>
        </div>
      )}
      {sortedAgents.map(({ key, findings, layer }) => {
        const visibleKeys = Object.keys(findings).filter(k => !isInternalKey(k))
        const layerLabel = layer !== undefined ? ` (L${layer})` : ''
        return (
          <div key={key}>
            <h4 className="text-sm font-medium text-foreground px-2 mb-1">{key}{layerLabel}</h4>
            <div className="border border-border rounded">
              {visibleKeys.map(k => (
                <FindingRow key={k} findingKey={k} value={findings[k]} />
              ))}
            </div>
          </div>
        )
      })}
    </div>
  )
}
