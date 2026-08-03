import { cn } from '@/lib/utils'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { Tooltip } from '@/components/ui/Tooltip'
import type { ToolCallDistributionEntry } from '@/types/session'

function ResultSplitBar({ entry }: { entry: ToolCallDistributionEntry }) {
  const total = entry.success + entry.error
  if (total <= 0) return null

  const successPct = (entry.success / total) * 100
  const errorPct = (entry.error / total) * 100

  const tooltip = (
    <div className="space-y-0.5">
      <div>Success: {entry.success} ({successPct.toFixed(0)}%)</div>
      <div>Error: {entry.error} ({errorPct.toFixed(0)}%)</div>
    </div>
  )

  return (
    <Tooltip text={tooltip}>
      <div className="flex w-full h-1.5 rounded-sm overflow-hidden">
        {entry.success > 0 && (
          <div className={cn('h-full', 'bg-green-500 dark:bg-green-400')} style={{ width: `${successPct}%` }} />
        )}
        {entry.error > 0 && (
          <div className={cn('h-full', 'bg-red-500 dark:bg-red-400')} style={{ width: `${errorPct}%` }} />
        )}
      </div>
    </Tooltip>
  )
}

/** Calls-per-tool table with a result-status split bar. Renders nothing for sessions with no recorded tool calls (legacy sessions predating this audit). */
export function SessionToolDistribution({ toolCalls }: { toolCalls?: ToolCallDistributionEntry[] }) {
  if (!toolCalls || toolCalls.length === 0) return null

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium">Tool calls</h3>
      <Table>
        <TableHeader>
          <TableRow className="bg-muted/30">
            <TableHead className="w-40">Tool</TableHead>
            <TableHead className="w-20">Calls</TableHead>
            <TableHead className="w-32">Result split</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {toolCalls.map((entry) => (
            <TableRow key={entry.tool_name} className="font-mono text-xs">
              <TableCell>{entry.tool_name}</TableCell>
              <TableCell className="text-muted-foreground">{entry.success + entry.error}</TableCell>
              <TableCell>
                <ResultSplitBar entry={entry} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
