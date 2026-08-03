import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { TableRow, TableCell } from '@/components/ui/Table'
import { HandoffDigestSection } from '@/components/workflow/HandoffDigestSection'
import { cn, formatDateTime } from '@/lib/utils'
import { fallbackLabel, formatCost, runAgentLabel, runStatusVariant, runTokens } from '@/lib/systemAgentRuns'
import type { SystemAgentRun } from '@/types/systemAgentRuns'

interface SystemAgentRunRowProps {
  run: SystemAgentRun
  nested?: boolean
}

export function TicketLink({ run }: { run: SystemAgentRun }) {
  if (run.ticket_id) {
    return (
      <Link to={`/tickets/${run.ticket_id}?tab=workflow`} className="text-primary hover:underline text-xs">
        {run.ticket_id.slice(0, 8)}
      </Link>
    )
  }
  return (
    <Link to="/project-workflows" className="text-primary hover:underline text-xs">
      {run.node_id || run.workflow_instance_id || '—'}
    </Link>
  )
}

export function SystemAgentRunRow({ run, nested }: SystemAgentRunRowProps) {
  const [expanded, setExpanded] = useState(false)
  const tokens = runTokens(run)
  const label = fallbackLabel(run)
  const isSession = run.kind === 'agent_session'
  const statusText = isSession ? run.result || run.status : run.status
  const failed = runStatusVariant(run) === 'destructive'

  return (
    <>
      <TableRow>
        <TableCell className={cn('text-xs', nested && 'pl-6')}>
          <div className="flex items-center gap-1">
            {isSession && (
              <Button
                variant="ghost"
                size="sm"
                className="h-5 w-5 p-0 shrink-0"
                onClick={() => setExpanded((prev) => !prev)}
                aria-label={expanded ? 'Collapse' : 'Expand'}
              >
                {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
              </Button>
            )}
            {runAgentLabel(run)}
          </div>
        </TableCell>
        <TableCell className="text-xs">
          {run.tier ? <Badge variant="outline">Tier {run.tier}</Badge> : '—'}
        </TableCell>
        <TableCell className="text-xs whitespace-nowrap">
          {run.resolved_provider || '—'} · {run.model_id || '—'} · {run.resolved_effort || run.resolved_execution_mode || '—'}
        </TableCell>
        <TableCell className="text-xs">
          {label ? <Badge variant={failed ? 'destructive' : 'secondary'}>{label}</Badge> : '—'}
        </TableCell>
        <TableCell className="text-xs whitespace-nowrap">
          {tokens.input} in / {tokens.output} out
        </TableCell>
        <TableCell className="text-xs">{formatCost(run.cost_estimate)}</TableCell>
        <TableCell className="text-xs">
          <div className="flex items-center gap-1">
            <Badge variant={runStatusVariant(run)}>{statusText || '—'}</Badge>
            {run.kind === 'refinery_fold' && run.error && (
              <span className="text-destructive text-xs">{run.error}</span>
            )}
          </div>
        </TableCell>
        <TableCell className="text-xs whitespace-nowrap">{formatDateTime(run.created_at)}</TableCell>
        <TableCell className="text-xs">
          <TicketLink run={run} />
          {run.project_id && <div className="text-muted-foreground">{run.project_id}</div>}
        </TableCell>
      </TableRow>
      {expanded && isSession && (
        <TableRow>
          <TableCell colSpan={9} className="bg-muted/10">
            <HandoffDigestSection sessionId={run.session_id} enabled={expanded} />
          </TableCell>
        </TableRow>
      )}
    </>
  )
}
