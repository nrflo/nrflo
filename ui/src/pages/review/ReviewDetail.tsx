import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Textarea } from '@/components/ui/Textarea'
import { MarkdownEditor } from '@/components/ui/MarkdownEditor'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { DiffPreview } from '@/components/review/DiffPreview'
import {
  useReviewItem,
  useUpdateReviewDraft,
  useApproveReview,
  useRejectReview,
} from '@/hooks/useReview'

const STATUS_BADGE: Record<string, 'secondary' | 'default' | 'destructive'> = {
  pending: 'secondary',
  approved: 'default',
  rejected: 'destructive',
}

export function ReviewDetailPage() {
  const { id = '' } = useParams<{ id: string }>()
  const { data: item, isLoading, error } = useReviewItem(id)
  const [draftText, setDraftText] = useState('')
  const [showRejectDialog, setShowRejectDialog] = useState(false)
  const [rejectReason, setRejectReason] = useState('')
  const updateDraft = useUpdateReviewDraft()
  const approve = useApproveReview()
  const reject = useRejectReview()

  if (isLoading) return <div className="p-6 text-center text-muted-foreground">Loading…</div>
  if (error || !item)
    return <div className="p-6 text-center text-destructive">Error loading review item.</div>

  const inputText = JSON.stringify(item.input, null, 2)
  const outputText = JSON.stringify(item.output, null, 2)
  const currentDraft = draftText || JSON.stringify(item.draft ?? item.output, null, 2)

  const handleSaveDraft = () => {
    try {
      const parsed = JSON.parse(currentDraft) as Record<string, unknown>
      updateDraft.mutate({ id, draft: parsed })
    } catch {
      // invalid JSON — no-op
    }
  }

  const handleReject = () => {
    reject.mutate({ id, reason: rejectReason }, { onSuccess: () => setShowRejectDialog(false) })
  }

  return (
    <div className="p-6 max-w-7xl mx-auto space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">{item.tool_name}</h2>
        <Badge variant={STATUS_BADGE[item.status] ?? 'secondary'}>{item.status}</Badge>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <Card>
          <CardHeader className="py-3">
            <CardTitle className="text-sm">Input</CardTitle>
          </CardHeader>
          <CardContent className="pt-0">
            <MarkdownEditor value={inputText} readOnly minHeight="200px" maxHeight="400px" />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="py-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm">Draft</CardTitle>
              <Button
                size="sm"
                variant="outline"
                onClick={handleSaveDraft}
                disabled={updateDraft.isPending}
              >
                Save draft
              </Button>
            </div>
          </CardHeader>
          <CardContent className="pt-0">
            <MarkdownEditor
              value={currentDraft}
              onChange={setDraftText}
              minHeight="200px"
              maxHeight="400px"
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="py-3">
            <CardTitle className="text-sm">Diff</CardTitle>
          </CardHeader>
          <CardContent className="pt-0">
            <DiffPreview before={outputText} after={currentDraft} />
          </CardContent>
        </Card>
      </div>

      {item.status === 'pending' && (
        <div className="flex gap-2">
          <Button onClick={() => approve.mutate(id)} disabled={approve.isPending}>
            Approve
          </Button>
          <Button variant="destructive" onClick={() => setShowRejectDialog(true)}>
            Reject
          </Button>
        </div>
      )}

      <Dialog open={showRejectDialog} onClose={() => setShowRejectDialog(false)}>
        <DialogHeader onClose={() => setShowRejectDialog(false)}>Reject Review</DialogHeader>
        <DialogBody>
          <Textarea
            placeholder="Reason for rejection…"
            value={rejectReason}
            onChange={(e) => setRejectReason(e.target.value)}
            className="min-h-[80px]"
          />
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => setShowRejectDialog(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleReject}
            disabled={reject.isPending || !rejectReason.trim()}
          >
            Reject
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  )
}
