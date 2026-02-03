import { useState, useCallback } from 'react'
import { ChevronDown, ChevronRight, CheckCircle, XCircle, Copy, Check, Cpu, Clock } from 'lucide-react'
import { cn, statusColor, formatDateTime } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import type { PhaseState, WorkflowFindings, ActiveAgentV4, AgentHistoryEntry } from '@/types/workflow'

interface PhaseCardProps {
  name: string
  phase: PhaseState
  isCurrent?: boolean
  findings?: WorkflowFindings
  activeAgents?: Record<string, ActiveAgentV4>
  agentHistory?: AgentHistoryEntry[]
}

function resultColor(result?: string | null): string {
  if (result === 'pass') return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200'
  if (result === 'fail') return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
  if (result === 'skipped') return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200'
  return ''
}

function ResultIcon({ result }: { result?: string | null }) {
  if (result === 'pass') return <CheckCircle className="h-3 w-3" />
  if (result === 'fail') return <XCircle className="h-3 w-3" />
  return null
}

// Reverse mapping: agent type to phase
function agentTypeToPhase(agentType: string): string | null {
  const mapping: Record<string, string> = {
    'setup-analyzer': 'investigation',
    'test-writer': 'test-design',
    'implementor': 'implementation',
    'qa-verifier': 'verification',
    'doc-updater': 'docs',
  }
  return mapping[agentType] || null
}


function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  const mins = Math.floor(seconds / 60)
  const secs = seconds % 60
  return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`
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

// Clickable agent badge for header - shows agent type with result color and expand chevron
interface AgentBadgeProps {
  agent: AgentHistoryEntry
  findings?: Record<string, unknown>
  expanded: boolean
  onToggle: () => void
}

function AgentBadge({ agent, findings, expanded, onToggle }: AgentBadgeProps) {
  const hasFindings = findings && Object.keys(findings).length > 0

  // Color based on result
  const badgeClass = cn(
    'inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-mono transition-colors',
    agent.result === 'pass' && 'bg-green-100 text-green-800 dark:bg-green-900/50 dark:text-green-200',
    agent.result === 'fail' && 'bg-red-100 text-red-800 dark:bg-red-900/50 dark:text-red-200',
    !agent.result && 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300',
    hasFindings && 'cursor-pointer hover:opacity-80'
  )

  return (
    <span
      role="button"
      tabIndex={hasFindings ? 0 : -1}
      className={badgeClass}
      onClick={(e) => {
        e.stopPropagation()
        if (hasFindings) onToggle()
      }}
      onKeyDown={(e) => {
        if (hasFindings && (e.key === 'Enter' || e.key === ' ')) {
          e.preventDefault()
          e.stopPropagation()
          onToggle()
        }
      }}
      title={hasFindings ? `${agent.agent_type} - click to ${expanded ? 'collapse' : 'expand'} findings` : agent.agent_type}
    >
      {hasFindings && (
        expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />
      )}
      <span>{agent.agent_type}</span>
      {agent.result && <ResultIcon result={agent.result} />}
    </span>
  )
}

// Inline findings display for expanded agent badges
interface InlineFindingsProps {
  agent: AgentHistoryEntry
  findings: Record<string, unknown>
}

function InlineFindings({ agent, findings }: InlineFindingsProps) {
  const [copied, setCopied] = useState(false)

  const copyFindings = (e: React.MouseEvent) => {
    e.stopPropagation()
    navigator.clipboard.writeText(JSON.stringify(findings, null, 2))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div
      className="mt-2 p-3 rounded-lg bg-muted/30 border border-border/50 space-y-2"
      onClick={(e) => e.stopPropagation()}
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Cpu className="h-3 w-3 text-purple-500" />
          <span className="font-mono text-xs font-medium">{agent.agent_type}</span>
          {agent.model_id && (
            <span className="text-xs text-muted-foreground">({agent.model_id})</span>
          )}
          {agent.duration_sec !== undefined && (
            <span className="text-xs text-muted-foreground flex items-center gap-1">
              <Clock className="h-3 w-3" />
              {formatDuration(agent.duration_sec)}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">
            {Object.keys(findings).length} field{Object.keys(findings).length !== 1 ? 's' : ''}
          </span>
          <Button variant="ghost" size="sm" className="h-5 px-1" onClick={copyFindings}>
            {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
          </Button>
        </div>
      </div>
      <div className="space-y-2 text-sm">
        {Object.entries(findings).map(([key, value]) => (
          <div key={key} className="space-y-1">
            <div className="flex items-center gap-2">
              <Badge variant="outline" className="text-xs font-mono shrink-0">{key}</Badge>
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
    </div>
  )
}

interface AgentHistoryCardProps {
  agent: AgentHistoryEntry
  findings?: Record<string, unknown>
}

function AgentHistoryCard({ agent, findings }: AgentHistoryCardProps) {
  const [expanded, setExpanded] = useState(false)
  const [copied, setCopied] = useState(false)
  const hasFindings = findings && Object.keys(findings).length > 0

  const copyFindings = () => {
    if (findings) {
      navigator.clipboard.writeText(JSON.stringify(findings, null, 2))
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  return (
    <div className="border border-border rounded-lg overflow-hidden">
      <button
        onClick={() => hasFindings && setExpanded(!expanded)}
        disabled={!hasFindings}
        className={cn(
          'w-full flex items-center gap-3 p-2 text-left transition-colors',
          hasFindings && 'hover:bg-muted/50 cursor-pointer',
          !hasFindings && 'cursor-default'
        )}
      >
        {hasFindings && (
          <span className="text-muted-foreground">
            {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          </span>
        )}
        <Cpu className="h-4 w-4 text-purple-500 shrink-0" />
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm">{agent.agent_type}</span>
            {agent.model_id && (
              <Badge variant="outline" className="text-xs">
                {agent.model_id}
              </Badge>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2">
          {agent.duration_sec !== undefined && (
            <span className="text-xs text-muted-foreground flex items-center gap-1">
              <Clock className="h-3 w-3" />
              {formatDuration(agent.duration_sec)}
            </span>
          )}
          {agent.result && (
            <Badge className={cn('text-xs flex items-center gap-1', resultColor(agent.result))}>
              <ResultIcon result={agent.result} />
              {agent.result}
            </Badge>
          )}
        </div>
      </button>

      {expanded && hasFindings && (
        <div className="p-3 border-t border-border bg-muted/20 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">
              {Object.keys(findings).length} field{Object.keys(findings).length !== 1 ? 's' : ''}
            </span>
            <Button variant="ghost" size="sm" className="h-5 px-1" onClick={copyFindings}>
              {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
            </Button>
          </div>
          <div className="space-y-2 text-sm">
            {Object.entries(findings).map(([key, value]) => (
              <div key={key} className="space-y-1">
                <div className="flex items-center gap-2">
                  <Badge variant="outline" className="text-xs font-mono shrink-0">{key}</Badge>
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
        </div>
      )}
    </div>
  )
}

// Mapping: phase name -> agent types that run in that phase
const phaseAgentMapping: Record<string, string[]> = {
  'investigation': ['setup-analyzer'],
  'test-design': ['test-writer'],
  'implementation': ['implementor'],
  'verification': ['test-writer', 'qa-verifier'],
  'docs': ['doc-updater'],
}

// Check if an object looks like it contains model keys (e.g., "claude:opus", "opencode:gpt")
// rather than actual finding keys (e.g., "l1_issues", "summary")
function isModelKeyedObject(obj: Record<string, unknown>): boolean {
  const keys = Object.keys(obj)
  if (keys.length === 0) return false
  // Model keys typically contain ":" (cli:model format) or start with known CLI prefixes
  return keys.every(key =>
    key.includes(':') ||
    key.startsWith('claude') ||
    key.startsWith('opencode') ||
    key.startsWith('openai')
  )
}

// Resolve findings for an agent - collects ALL findings including from nested model structures
// Format 1 (composite key): findings["agent_type:model_id"] = { key: value }
// Format 2 (model-keyed): findings[agent_type][model_id] = { key: value }
// Format 3 (direct): findings[agent_type] = { key: value }
function resolveAgentFindings(
  findings: WorkflowFindings | undefined,
  agentType: string,
  modelId?: string
): Record<string, unknown> | undefined {
  if (!findings) return undefined

  // Format 1: Try composite key first (agent_type:model_id)
  if (modelId) {
    const compositeKey = `${agentType}:${modelId}`
    const compositeFindings = findings[compositeKey]
    if (compositeFindings && typeof compositeFindings === 'object') {
      return compositeFindings as Record<string, unknown>
    }
  }

  const agentFindings = findings[agentType]
  if (!agentFindings || typeof agentFindings !== 'object') return undefined

  // Format 2: Try model-specific nested findings
  if (modelId) {
    const modelFindings = agentFindings[modelId]
    if (modelFindings && typeof modelFindings === 'object' && !Array.isArray(modelFindings)) {
      return modelFindings as Record<string, unknown>
    }
  }

  // Check if agentFindings is model-keyed (contains model IDs as keys, not actual findings)
  if (isModelKeyedObject(agentFindings as Record<string, unknown>)) {
    // Merge all findings from all models into a single object
    // Prefix keys with model ID to avoid collisions and show source
    const merged: Record<string, unknown> = {}
    for (const [modelKey, modelData] of Object.entries(agentFindings)) {
      if (modelData && typeof modelData === 'object' && !Array.isArray(modelData)) {
        for (const [findingKey, findingValue] of Object.entries(modelData as Record<string, unknown>)) {
          // If there's only one model, don't prefix. Otherwise prefix with model name.
          const keys = Object.keys(agentFindings)
          if (keys.length === 1) {
            merged[findingKey] = findingValue
          } else {
            // Use short model name (e.g., "opus" from "claude:opus")
            const shortModel = modelKey.includes(':') ? modelKey.split(':')[1] : modelKey
            merged[`[${shortModel}] ${findingKey}`] = findingValue
          }
        }
      }
    }
    return Object.keys(merged).length > 0 ? merged : undefined
  }

  // Format 3: Return the agentFindings directly (it contains actual findings)
  return agentFindings as Record<string, unknown>
}

export function PhaseCard({ name, phase, isCurrent, findings, activeAgents, agentHistory }: PhaseCardProps) {
  const [expanded, setExpanded] = useState(isCurrent || phase.status === 'error')
  // Track which agent badges are expanded (by agent_id)
  const [expandedAgents, setExpandedAgents] = useState<Set<string>>(new Set())
  const hasDetails = phase.error || phase.started_at || phase.ended_at || phase.result

  // Filter agent history for this phase
  let phaseAgentHistory = agentHistory?.filter(h => h.phase === name) || []

  // Fallback: if no agent_history but we have findings, derive agents from findings
  // using the phase-to-agent mapping
  if (phaseAgentHistory.length === 0 && findings) {
    const expectedAgentTypes = phaseAgentMapping[name] || []
    phaseAgentHistory = expectedAgentTypes
      .filter(agentType => findings[agentType] && Object.keys(findings[agentType]).length > 0)
      .map(agentType => ({
        agent_id: `${name}-${agentType}`, // synthetic ID
        agent_type: agentType,
        phase: name,
        result: phase.result === 'pass' ? 'pass' : phase.result === 'fail' ? 'fail' : undefined,
      }))
  }

  const hasAgentHistory = phaseAgentHistory.length > 0

  // Check if any agents in this phase have findings
  const phaseHasFindings = findings && phaseAgentHistory.some(agent => {
    const agentFindings = resolveAgentFindings(findings, agent.agent_type, agent.model_id)
    return agentFindings && Object.keys(agentFindings).length > 0
  })

  // Filter active agents for this phase
  const phaseActiveAgents = activeAgents
    ? Object.entries(activeAgents).filter(([, agent]) => {
        const agentPhase = agentTypeToPhase(agent.agent_type)
        return agentPhase === name
      })
    : []
  const hasActiveAgents = phaseActiveAgents.length > 0

  // Allow expansion if there's any content to show
  const canExpand = hasDetails || hasAgentHistory || hasActiveAgents || phaseHasFindings

  // Toggle expansion state for an agent badge
  const toggleAgentExpansion = useCallback((agentId: string) => {
    setExpandedAgents(prev => {
      const next = new Set(prev)
      if (next.has(agentId)) {
        next.delete(agentId)
      } else {
        next.add(agentId)
      }
      return next
    })
  }, [])

  return (
    <div
      className={cn(
        'rounded-lg border p-3',
        isCurrent && 'border-primary bg-primary/5',
        phase.status === 'error' && 'border-red-500 bg-red-50 dark:bg-red-950'
      )}
    >
      <div className="space-y-0">
        <button
          className={cn(
            'flex items-center gap-2 w-full text-left',
            canExpand && 'cursor-pointer'
          )}
          onClick={() => canExpand && setExpanded(!expanded)}
          disabled={!canExpand}
        >
          {canExpand && (
            <span className="text-muted-foreground">
              {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            </span>
          )}
          <h3 className="font-medium capitalize">
            {name.replace(/_/g, ' ')}
          </h3>
          {/* Agent badges - individual clickable badges for each agent */}
          {hasAgentHistory && (
            <div className="flex items-center gap-1 flex-wrap">
              {phaseAgentHistory.map((agent) => (
                <AgentBadge
                  key={agent.agent_id}
                  agent={agent}
                  findings={resolveAgentFindings(findings, agent.agent_type, agent.model_id)}
                  expanded={expandedAgents.has(agent.agent_id)}
                  onToggle={() => toggleAgentExpansion(agent.agent_id)}
                />
              ))}
            </div>
          )}
          <div className="flex items-center gap-2 ml-auto">
            {phase.result && (
              <Badge className={cn('text-xs flex items-center gap-1', resultColor(phase.result))}>
                <ResultIcon result={phase.result} />
                {phase.result}
              </Badge>
            )}
            <Badge className={cn('text-xs', statusColor(phase.status))}>
              {phase.status.replace(/_/g, ' ')}
            </Badge>
          </div>
        </button>

        {/* Inline findings for expanded agent badges (shown even when phase is collapsed) */}
        {expandedAgents.size > 0 && (
          <div className="pl-6 space-y-2">
            {phaseAgentHistory
              .filter(agent => expandedAgents.has(agent.agent_id))
              .map(agent => {
                const agentFindings = resolveAgentFindings(findings, agent.agent_type, agent.model_id)
                if (!agentFindings || Object.keys(agentFindings).length === 0) return null
                return (
                  <InlineFindings
                    key={agent.agent_id}
                    agent={agent}
                    findings={agentFindings}
                  />
                )
              })}
          </div>
        )}
      </div>

      {expanded && canExpand && (
        <div className="mt-3 pt-3 border-t border-border/50 space-y-3">
          {/* Phase details summary */}
          <div className="flex flex-wrap gap-x-6 gap-y-2 text-sm">
            {phase.result && (
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground">Result:</span>
                <Badge className={cn('text-xs flex items-center gap-1', resultColor(phase.result))}>
                  <ResultIcon result={phase.result} />
                  {phase.result}
                </Badge>
              </div>
            )}
            {phase.started_at && (
              <div>
                <span className="text-muted-foreground">Started:</span>{' '}
                <span className="text-xs">{formatDateTime(phase.started_at)}</span>
              </div>
            )}
            {phase.ended_at && (
              <div>
                <span className="text-muted-foreground">Ended:</span>{' '}
                <span className="text-xs">{formatDateTime(phase.ended_at)}</span>
              </div>
            )}
          </div>

          {/* Error */}
          {phase.error && (
            <div className="p-2 bg-red-100 dark:bg-red-900/50 rounded text-sm text-red-800 dark:text-red-200">
              <strong>Error:</strong> {phase.error}
            </div>
          )}

          {/* Active Agents */}
          {hasActiveAgents && (
            <div className="space-y-2">
              <div className="text-xs font-medium text-muted-foreground">Running:</div>
              {phaseActiveAgents.map(([key, agent]) => (
                <div
                  key={key}
                  className="flex items-center gap-3 p-2 rounded bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800"
                >
                  <Cpu className="h-4 w-4 text-yellow-600 dark:text-yellow-400 animate-pulse shrink-0" />
                  <div className="flex-1 min-w-0">
                    <div className="font-mono text-xs">{agent.agent_type}</div>
                    {agent.model && (
                      <div className="text-xs text-muted-foreground truncate">{agent.model}</div>
                    )}
                  </div>
                  <div className="text-xs text-muted-foreground text-right">
                    {agent.pid && <div>PID: {agent.pid}</div>}
                    {agent.started_at && <div>{formatDateTime(agent.started_at)}</div>}
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* Agent History with findings */}
          {hasAgentHistory && (
            <div className="space-y-2">
              <div className="text-xs font-medium text-muted-foreground">Agents:</div>
              {phaseAgentHistory.map((agent) => (
                <AgentHistoryCard
                  key={agent.agent_id}
                  agent={agent}
                  findings={resolveAgentFindings(findings, agent.agent_type, agent.model_id)}
                />
              ))}
            </div>
          )}


        </div>
      )}
    </div>
  )
}
