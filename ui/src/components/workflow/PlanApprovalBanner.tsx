import { useState } from 'react'
import { toast } from 'sonner'
import { ClipboardList, XCircle, Pencil } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { usePlan, useApprovePlan, useCancelPlan } from '@/hooks/usePlan'
import { ApiError } from '@/api/client'
import { planStatusLabel } from '@/lib/utils'
import { PlanManifestView } from './PlanManifestView'
import { PlanReviseDialog } from './PlanReviseDialog'
import type { WorkflowInstanceStatus } from '@/types/workflow'

interface PlanApprovalBannerProps {
  instanceId: string
  status: WorkflowInstanceStatus
}

export function reportPlanError(action: string, err: unknown) {
  if (err instanceof ApiError && err.status === 409) {
    toast.error('The plan changed since this page loaded — reload to see the latest revision.')
    return
  }
  const message = err instanceof Error ? err.message : String(err)
  toast.error(`Failed to ${action} plan: ${message}`)
}

// The plan-suspended counterpart of WorkflowPauseControls: shown whenever the
// instance status is one of PLAN_SUSPENDED_STATUSES.
export function PlanApprovalBanner({ instanceId, status }: PlanApprovalBannerProps) {
  const [cancelConfirmOpen, setCancelConfirmOpen] = useState(false)
  const [reviseOpen, setReviseOpen] = useState(false)
  const { data: draft, isLoading } = usePlan(instanceId)
  const approveMutation = useApprovePlan()
  const cancelMutation = useCancelPlan()

  const head = draft?.head
  const manifest = draft?.manifest

  const handleApprove = () => {
    if (!head) return
    approveMutation.mutate(
      { instanceId, params: { revision: head.latest_revision } },
      { onError: (err) => reportPlanError('approve', err) }
    )
  }

  const handleCancel = () => {
    cancelMutation.mutate(
      { instanceId },
      { onError: (err) => reportPlanError('cancel', err) }
    )
  }

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm dark:border-amber-800 dark:bg-amber-950/30">
      <div className="flex items-center gap-2 text-amber-700 dark:text-amber-400">
        <ClipboardList className="h-4 w-4" />
        <Badge className="bg-amber-500/20 text-amber-700 dark:text-amber-400 border-amber-500/30">
          {planStatusLabel(status) ?? status}
        </Badge>
        <span className="font-medium">Plan-driven run — awaiting review</span>
      </div>

      {isLoading ? (
        <Spinner size="sm" />
      ) : manifest ? (
        <PlanManifestView manifest={manifest} questions={draft?.questions} />
      ) : (
        <p className="text-muted-foreground">No plan draft yet.</p>
      )}

      {head && (
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            onClick={handleApprove}
            disabled={approveMutation.isPending || head.status !== 'draft'}
          >
            {approveMutation.isPending ? <Spinner size="sm" className="mr-2" /> : null}
            Approve (rev {head.latest_revision})
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setReviseOpen(true)}
            disabled={head.status !== 'draft'}
          >
            <Pencil className="h-4 w-4 mr-2" />
            Revise
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setCancelConfirmOpen(true)}
            disabled={cancelMutation.isPending || head.status !== 'draft'}
            className="text-destructive hover:text-destructive"
          >
            {cancelMutation.isPending ? <Spinner size="sm" className="mr-2" /> : <XCircle className="h-4 w-4 mr-2" />}
            Cancel Plan
          </Button>
        </div>
      )}

      <ConfirmDialog
        open={cancelConfirmOpen}
        onClose={() => setCancelConfirmOpen(false)}
        onConfirm={handleCancel}
        title="Cancel Plan"
        message="Cancel this draft plan? The workflow instance will remain parked until a new plan is drafted."
        confirmLabel="Cancel Plan"
        variant="destructive"
      />

      {reviseOpen && head && (
        <PlanReviseDialog
          onClose={() => setReviseOpen(false)}
          instanceId={instanceId}
          revision={head.latest_revision}
          questions={draft?.questions}
        />
      )}
    </div>
  )
}
