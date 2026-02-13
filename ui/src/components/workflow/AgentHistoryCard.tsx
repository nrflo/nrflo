import { useState } from 'react'
import { ChevronDown, ChevronRight, Copy, Check, Cpu, Clock, Timer } from 'lucide-react'
import { cn, formatElapsedTime, contextLeftColor } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { resultColor, ResultIcon, formatDuration } from './resultUtils'
import { SimpleFindingValue } from './findingUtils'
import type { AgentHistoryEntry } from '@/types/workflow'

// Inline findings display for expanded agent badges
interface InlineFindingsProps {
  agent: AgentHistoryEntry
  findings: Record<string, unknown>
}

export function InlineFindings({ agent, findings }: InlineFindingsProps) {
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

export function AgentHistoryCard({ agent, findings }: AgentHistoryCardProps) {
  const [expanded, setExpanded] = useState(false)
  const [copied, setCopied] = useState(false)
  const hasFindings = findings && Object.keys(findings).length > 0

  const durationText = agent.started_at && agent.ended_at
    ? formatElapsedTime(agent.started_at, agent.ended_at)
    : agent.duration_sec
      ? formatDuration(agent.duration_sec)
      : undefined

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
            {(agent.restart_count ?? 0) > 0 && (
              <span className="text-xs font-mono px-1 rounded bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400">
                ↻{agent.restart_count}
              </span>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2">
          {durationText && (
            <span className="text-xs text-muted-foreground flex items-center gap-1">
              <Timer className="h-3 w-3" />
              {durationText}
            </span>
          )}
          {agent.context_left != null && (
            <span className={cn(
              'text-xs font-mono px-1.5 py-0.5 rounded',
              contextLeftColor(agent.context_left)
            )}>
              {agent.context_left}% ctx
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
