import { useState } from 'react'
import { ChevronRight, ChevronDown } from 'lucide-react'
import type { WorkflowFindings } from '@/types/workflow'

interface FindingsPanelProps {
  projectFindings: Record<string, unknown> | undefined
  agentFindings: WorkflowFindings | undefined
  selectedAgentType: string | null
}

export function formatValue(value: unknown): { text: string; isJson: boolean } {
  if (value === null || value === undefined) return { text: String(value), isJson: false }
  if (typeof value === 'object') {
    return { text: JSON.stringify(value, null, 2), isJson: true }
  }
  const str = String(value)
  // Try parsing string values as JSON
  try {
    const parsed = JSON.parse(str)
    if (typeof parsed === 'object' && parsed !== null) {
      return { text: JSON.stringify(parsed, null, 2), isJson: true }
    }
  } catch {
    // not JSON
  }
  return { text: str, isJson: false }
}

export function FindingRow({ findingKey, value }: { findingKey: string; value: unknown }) {
  const [expanded, setExpanded] = useState(false)
  const { text, isJson } = formatValue(value)
  const isLong = text.length > 80 || text.includes('\n')

  return (
    <div className="border-b border-border/50 last:border-b-0 min-w-0">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-1.5 w-full px-2 py-1.5 text-left hover:bg-muted/50 transition-colors"
      >
        {isLong ? (
          expanded ? <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" /> : <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
        ) : (
          <span className="w-3 shrink-0" />
        )}
        <span className="text-xs font-medium text-foreground">{findingKey}</span>
        {!expanded && !isLong && (
          <span className="text-xs font-mono text-muted-foreground truncate ml-2">{text}</span>
        )}
      </button>
      {expanded && (
        <div className="px-2 pb-2 pl-7 overflow-hidden">
          {isJson ? (
            <pre className="text-xs font-mono text-foreground/80 whitespace-pre-wrap break-words bg-muted/30 rounded p-2">{text}</pre>
          ) : (
            <p className="text-xs font-mono text-foreground/80 whitespace-pre-wrap break-words">{text}</p>
          )}
        </div>
      )}
    </div>
  )
}

export function isInternalKey(key: string): boolean {
  return key.startsWith('_')
}

export function FindingsPanel({ projectFindings, agentFindings, selectedAgentType }: FindingsPanelProps) {
  const projectKeys = projectFindings ? Object.keys(projectFindings).filter(k => !isInternalKey(k)) : []
  const hasProjectFindings = projectKeys.length > 0

  // Filter agent findings
  const agentEntries = agentFindings
    ? Object.entries(agentFindings).filter(([key]) => !isInternalKey(key))
    : []
  const filteredAgents = selectedAgentType
    ? agentEntries.filter(([key]) => key === selectedAgentType || key.startsWith(selectedAgentType + ':'))
    : agentEntries

  const hasAgentFindings = filteredAgents.some(([, findings]) =>
    Object.keys(findings).filter(k => !isInternalKey(k)).length > 0
  )

  if (!hasProjectFindings && !hasAgentFindings) {
    return (
      <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
        <p className="text-xs">No findings available</p>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {hasProjectFindings && (
        <div>
          <h4 className="text-sm font-medium text-foreground px-2 mb-1">Project Findings</h4>
          <div className="border border-border rounded">
            {projectKeys.map(key => (
              <FindingRow key={key} findingKey={key} value={projectFindings![key]} />
            ))}
          </div>
        </div>
      )}
      {filteredAgents.map(([agentType, findings]) => {
        const visibleKeys = Object.keys(findings).filter(k => !isInternalKey(k))
        if (visibleKeys.length === 0) return null
        return (
          <div key={agentType}>
            <h4 className="text-sm font-medium text-foreground px-2 mb-1">{agentType}</h4>
            <div className="border border-border rounded">
              {visibleKeys.map(key => (
                <FindingRow key={key} findingKey={key} value={findings[key]} />
              ))}
            </div>
          </div>
        )
      })}
    </div>
  )
}
