import { useState } from 'react'
import { Dropdown } from '@/components/ui/Dropdown'
import { Spinner } from '@/components/ui/Spinner'
import { Table, TableHeader, TableBody, TableRow, TableHead } from '@/components/ui/Table'
import { useSystemAgentRuns } from '@/hooks/useSystemAgentRuns'
import { SystemAgentRunRow } from './SystemAgentRunRow'

const LIMIT_OPTIONS = [
  { value: '50', label: '50 rows' },
  { value: '100', label: '100 rows' },
  { value: '200', label: '200 rows' },
]

export function SystemAgentRunsSection() {
  const [limit, setLimit] = useState(50)
  const { data, isLoading, error } = useSystemAgentRuns(limit)
  const items = data?.items ?? []

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold">Activity</h2>
        <p className="text-sm text-muted-foreground">
          Merged tier/system-agent sessions, refinery folds, and stepwise step rotations across all projects.
        </p>
      </div>

      <div className="flex items-center gap-3 flex-wrap">
        <Dropdown value={String(limit)} onChange={(v) => setLimit(Number(v))} options={LIMIT_OPTIONS} className="w-40" />
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <p className="text-destructive text-sm">
          {error instanceof Error ? error.message : 'Failed to load system agent runs'}
        </p>
      ) : items.length === 0 ? (
        <p className="text-center py-12 text-muted-foreground">No system agent activity found.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Agent</TableHead>
              <TableHead>Tier</TableHead>
              <TableHead>Provider · Model · Effort</TableHead>
              <TableHead>Fallback</TableHead>
              <TableHead>Tokens</TableHead>
              <TableHead>Cost</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>When</TableHead>
              <TableHead>Session</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((run) => (
              <SystemAgentRunRow key={`${run.kind}:${run.session_id}:${run.created_at}`} run={run} />
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}
