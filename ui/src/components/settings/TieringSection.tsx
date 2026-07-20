import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { useApplyTiering, useTieringReport } from '@/hooks/useTiering'
import type { TieringDefRow, TieringProjectReport } from '@/types/tiering'

function formatDelta(value: number | null | undefined) {
  if (value === null || value === undefined) return '—'
  const sign = value > 0 ? '+' : ''
  return `${sign}$${value.toFixed(2)}/mo`
}

// A def is applicable when the BE report attached no skip reason
// (consultant/hotfix/non-static/customized all populate skip_reason).
function isApplicable(def: TieringDefRow) {
  return !def.skip_reason
}

function defStatus(def: TieringDefRow): { label: string; variant: 'success' | 'secondary' | 'outline' } {
  switch (def.skip_reason) {
    case 'consultant':
      return { label: 'Consultant', variant: 'outline' }
    case 'hotfix':
      return { label: 'Hotfix — skip', variant: 'outline' }
    case 'non-static':
      return { label: 'Not a worker', variant: 'outline' }
    case 'customized':
      return { label: 'Customized — skip', variant: 'secondary' }
    default:
      return { label: 'Applicable', variant: 'success' }
  }
}

function TieringProjectTable({ project }: { project: TieringProjectReport }) {
  const applyTiering = useApplyTiering()
  const applicableCount = project.defs.filter(isApplicable).length
  const pendingProjectId = applyTiering.variables?.confirmations?.[0]?.project_id
  const isThisProjectPending = applyTiering.isPending && pendingProjectId === project.project_id
  const applyFailed = applyTiering.isError && pendingProjectId === project.project_id

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold">{project.project_name}</h3>
          <p className="text-xs text-muted-foreground">{formatDelta(project.est_monthly_delta)}</p>
        </div>
        <Button
          size="sm"
          onClick={() =>
            applyTiering.mutate({ confirmations: [{ project_id: project.project_id, confirm_all: true }] })
          }
          disabled={applyTiering.isPending || applicableCount === 0}
        >
          {isThisProjectPending ? 'Applying…' : `Apply (${applicableCount})`}
        </Button>
      </div>

      {applyFailed && (
        <p className="text-destructive text-xs">
          {applyTiering.error instanceof Error ? applyTiering.error.message : 'Apply failed'}
        </p>
      )}

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Def</TableHead>
            <TableHead>Role</TableHead>
            <TableHead>Current</TableHead>
            <TableHead>Recommended</TableHead>
            <TableHead>Δ/mo</TableHead>
            <TableHead>Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {project.defs.map((def) => {
            const status = defStatus(def)
            return (
              <TableRow key={`${def.workflow_id}/${def.def_id}`}>
                <TableCell className="text-sm">{def.def_id}</TableCell>
                <TableCell className="text-xs text-muted-foreground">{def.role}</TableCell>
                <TableCell className="text-xs whitespace-nowrap">
                  {def.current_model} / {def.current_effort || '—'}
                </TableCell>
                <TableCell className="text-xs whitespace-nowrap">
                  {def.recommended_model} / {def.recommended_effort || '—'}
                  <span className="text-muted-foreground"> · {def.recommended_template}</span>
                  {def.grants_delegation && <span className="text-muted-foreground"> +delegation</span>}
                </TableCell>
                <TableCell className="text-xs">{formatDelta(def.est_monthly_delta)}</TableCell>
                <TableCell>
                  <Badge variant={status.variant} title={def.skip_reason ?? undefined}>
                    {status.label}
                  </Badge>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}

export function TieringSection() {
  const { data, isLoading, error } = useTieringReport()
  const projects = data?.projects ?? []
  const knownDeltas = projects
    .map((p) => p.est_monthly_delta)
    .filter((d): d is number => d !== null && d !== undefined)
  const totalDelta = knownDeltas.length ? knownDeltas.reduce((a, b) => a + b, 0) : null

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Tiering</h2>
        <p className="text-sm text-muted-foreground">
          Dry-run agent model/effort tier recommendations across projects.
          {data && ` Estimated total delta: ${formatDelta(totalDelta)}`}
        </p>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <p className="text-destructive text-sm">
          {error instanceof Error ? error.message : 'Failed to load tiering report'}
        </p>
      ) : projects.length === 0 ? (
        <p className="text-center py-12 text-muted-foreground">No projects to report on.</p>
      ) : (
        <div className="space-y-6">
          {projects.map((project) => (
            <TieringProjectTable key={project.project_id} project={project} />
          ))}
        </div>
      )}
    </div>
  )
}
