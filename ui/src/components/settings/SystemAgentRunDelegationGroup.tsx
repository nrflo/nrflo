import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronDown, ChevronRight, GitBranch } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { TableRow, TableCell } from '@/components/ui/Table'
import { Tooltip } from '@/components/ui/Tooltip'
import { formatDateTime } from '@/lib/utils'
import { formatCost } from '@/lib/systemAgentRuns'
import type { SystemAgentRunDelegationGroup as DelegationGroup } from '@/lib/systemAgentRunGroups'
import { SystemAgentRunRow } from './SystemAgentRunRow'

function CallerSessionLink({ group }: { group: DelegationGroup }) {
  const anchorRun = group.workers[0]
  const label = (group.caller_session_id || anchorRun.session_id).slice(0, 8)
  if (anchorRun.ticket_id) {
    return (
      <Link to={`/tickets/${anchorRun.ticket_id}?tab=workflow`} className="text-primary hover:underline text-xs">
        {label}
      </Link>
    )
  }
  return (
    <Link to="/project-workflows" className="text-primary hover:underline text-xs">
      {label}
    </Link>
  )
}

function groupStatusVariant(status?: string): 'success' | 'destructive' | 'secondary' {
  if (status === 'failed') return 'destructive'
  if (status === 'completed') return 'success'
  return 'secondary'
}

interface SystemAgentRunDelegationGroupProps {
  group: DelegationGroup
}

export function SystemAgentRunDelegationGroup({ group }: SystemAgentRunDelegationGroupProps) {
  const [expanded, setExpanded] = useState(false)
  const workersShown = group.workers.length

  return (
    <>
      <TableRow className="bg-muted/5">
        <TableCell className="text-xs">
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              className="h-5 w-5 p-0 shrink-0"
              onClick={() => setExpanded((prev) => !prev)}
              aria-label={expanded ? 'Collapse' : 'Expand'}
            >
              {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
            </Button>
            {workersShown} of {group.fanout} workers
            {group.branch && (
              <Tooltip text={`Branch: ${group.branch}`} placement="top">
                <div className="flex items-center gap-1 text-muted-foreground">
                  <GitBranch className="h-3.5 w-3.5" />
                  <span className="truncate max-w-24" title={group.branch}>
                    {group.branch}
                  </span>
                </div>
              </Tooltip>
            )}
          </div>
        </TableCell>
        <TableCell className="text-xs">
          {group.delegate_tier ? <Badge variant="outline">{group.delegate_tier}</Badge> : '—'}
        </TableCell>
        <TableCell className="text-xs">—</TableCell>
        <TableCell className="text-xs">—</TableCell>
        <TableCell className="text-xs whitespace-nowrap">
          {group.input_tokens} in / {group.output_tokens} out
        </TableCell>
        <TableCell className="text-xs">{formatCost(group.cost_estimate)}</TableCell>
        <TableCell className="text-xs">
          <Badge variant={groupStatusVariant(group.status)}>{group.status || '—'}</Badge>
        </TableCell>
        <TableCell className="text-xs whitespace-nowrap">{formatDateTime(group.created_at)}</TableCell>
        <TableCell className="text-xs">
          <CallerSessionLink group={group} />
        </TableCell>
      </TableRow>
      {expanded &&
        group.workers.map((run) => (
          <SystemAgentRunRow key={`${run.kind}:${run.session_id}:${run.created_at}`} run={run} nested />
        ))}
    </>
  )
}
