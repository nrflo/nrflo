import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { formatCost } from '@/lib/systemAgentRuns'
import type { SessionStatsResponse } from '@/types/session'

/** Self-vs-subtree cost/token attribution for the flow tree rooted at this session. */
export function SessionCostRollup({ stats }: { stats?: SessionStatsResponse }) {
  if (!stats) return null

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium">Cost & tokens</h3>
      <Table>
        <TableHeader>
          <TableRow className="bg-muted/30">
            <TableHead className="w-24">Scope</TableHead>
            <TableHead className="w-24">Cost</TableHead>
            <TableHead className="w-24">Tokens</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow className="font-mono text-xs">
            <TableCell>Self</TableCell>
            <TableCell className="text-muted-foreground">{formatCost(stats.self_cost_usd)}</TableCell>
            <TableCell className="text-muted-foreground">{stats.self_tokens.toLocaleString()}</TableCell>
          </TableRow>
          <TableRow className="font-mono text-xs">
            <TableCell>Subtree</TableCell>
            <TableCell className="text-muted-foreground">{formatCost(stats.subtree_cost_usd)}</TableCell>
            <TableCell className="text-muted-foreground">{stats.subtree_tokens.toLocaleString()}</TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  )
}
