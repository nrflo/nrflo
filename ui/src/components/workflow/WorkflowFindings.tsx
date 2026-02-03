import { useState } from 'react'
import { ChevronDown, ChevronRight, FileText, Copy, Check, Cpu } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import type { WorkflowFindings as WorkflowFindingsType } from '@/types/workflow'

interface WorkflowFindingsProps {
  findings: WorkflowFindingsType
}

// Try to parse a string as JSON, returning the formatted string or null if not JSON
function tryFormatAsJson(value: string): string | null {
  try {
    const parsed = JSON.parse(value)
    return JSON.stringify(parsed, null, 2)
  } catch {
    return null
  }
}

// Simple finding value renderer - shows strings as-is or JSON-formatted for objects
function SimpleFindingValue({ value }: { value: unknown }): React.ReactNode {
  // If it's a string, try to parse as JSON for pretty formatting
  if (typeof value === 'string') {
    const formatted = tryFormatAsJson(value)
    if (formatted !== null) {
      return (
        <pre className="text-xs font-mono whitespace-pre-wrap break-words">
          {formatted}
        </pre>
      )
    }
    // Not valid JSON, show as-is (no truncation)
    return (
      <span className="text-green-700 dark:text-green-400 whitespace-pre-wrap break-words">
        {value}
      </span>
    )
  }

  // For objects/arrays, stringify to JSON
  if (typeof value === 'object' && value !== null) {
    return (
      <pre className="text-xs font-mono whitespace-pre-wrap break-words">
        {JSON.stringify(value, null, 2)}
      </pre>
    )
  }

  // For primitives (number, boolean, null)
  if (value === null || value === undefined) {
    return <span className="text-muted-foreground italic">null</span>
  }
  return <span>{String(value)}</span>
}

interface AgentFindingsProps {
  agentType: string
  findings: Record<string, unknown>
}

function AgentFindings({ agentType, findings }: AgentFindingsProps) {
  const [expanded, setExpanded] = useState(true)
  const [copied, setCopied] = useState(false)
  const findingEntries = Object.entries(findings)

  return (
    <div className="border border-border rounded-lg overflow-hidden">
      {/* Header */}
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-2 bg-muted/30 hover:bg-muted/50 transition-colors"
      >
        {expanded ? (
          <ChevronDown className="h-4 w-4 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-4 w-4 text-muted-foreground" />
        )}
        <Cpu className="h-4 w-4 text-purple-500" />
        <span className="font-medium text-sm">{agentType}</span>
        <Badge variant="secondary" className="text-xs ml-auto">
          {findingEntries.length} field{findingEntries.length !== 1 ? 's' : ''}
        </Badge>
        <Button
          variant="ghost"
          size="sm"
          className="h-6 px-2"
          onClick={(e) => {
            e.stopPropagation()
            navigator.clipboard.writeText(JSON.stringify(findings, null, 2))
            setCopied(true)
            setTimeout(() => setCopied(false), 2000)
          }}
        >
          {copied ? (
            <Check className="h-3 w-3" />
          ) : (
            <Copy className="h-3 w-3" />
          )}
        </Button>
      </button>

      {/* Content */}
      {expanded && (
        <div className="p-3 space-y-3 text-sm">
          {findingEntries.map(([key, value]) => (
            <div key={key} className="space-y-1">
              <div className="flex items-center gap-2">
                <Badge variant="outline" className="text-xs font-mono shrink-0">
                  {key}
                </Badge>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-5 w-5 p-0 ml-auto opacity-50 hover:opacity-100"
                  onClick={(e) => {
                    e.stopPropagation()
                    const text = typeof value === 'string'
                      ? value
                      : JSON.stringify(value, null, 2)
                    navigator.clipboard.writeText(text)
                  }}
                >
                  <Copy className="h-3 w-3" />
                </Button>
              </div>
              <div className="pl-2 border-l-2 border-border">
                <SimpleFindingValue value={value} />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export function WorkflowFindings({ findings }: WorkflowFindingsProps) {
  const [expanded, setExpanded] = useState(true)
  const [copied, setCopied] = useState(false)
  const agentEntries = Object.entries(findings)

  if (agentEntries.length === 0) {
    return null
  }

  const copyAll = async () => {
    await navigator.clipboard.writeText(JSON.stringify(findings, null, 2))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="mt-6 pt-4 border-t border-border">
      <div className="flex items-center justify-between mb-3">
        <button
          onClick={() => setExpanded(!expanded)}
          className="flex items-center gap-2 hover:text-foreground transition-colors"
        >
          {expanded ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
          <FileText className="h-4 w-4 text-blue-500" />
          <h4 className="text-sm font-medium">Workflow Findings</h4>
          <Badge variant="secondary" className="text-xs">
            {agentEntries.length} agent{agentEntries.length !== 1 ? 's' : ''}
          </Badge>
        </button>
        <Button variant="ghost" size="sm" onClick={copyAll}>
          {copied ? (
            <Check className="h-3 w-3 mr-1" />
          ) : (
            <Copy className="h-3 w-3 mr-1" />
          )}
          Copy All
        </Button>
      </div>

      {expanded && (
        <div className="space-y-3">
          {agentEntries.map(([agentType, agentFindings]) => (
            <AgentFindings
              key={agentType}
              agentType={agentType}
              findings={agentFindings as Record<string, unknown>}
            />
          ))}
        </div>
      )}
    </div>
  )
}
