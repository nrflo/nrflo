import { useState } from 'react'
import { ChevronDown, ChevronRight, FileStack } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { RenderedMarkdown } from '@/components/ui/RenderedMarkdown'
import { useSessionHandoffDigest } from '@/hooks/useSessionHandoffDigest'

interface HandoffDigestSectionProps {
  sessionId: string | undefined
  enabled: boolean
}

function formatUpdatedAt(updatedAt: string): string {
  const date = new Date(updatedAt)
  if (Number.isNaN(date.getTime())) return updatedAt
  return date.toLocaleString()
}

// Collapsible 'Handoff digest' section for the Ledger tab: shows the current
// autonomous-slot digest (content + fold telemetry), hydrated from
// GET /api/v1/sessions/{id}/handoff-digest and kept live via the WS
// agent.handoff_digest event. Renders null when there is no digest — the
// digest is durable, so this is only absent for sessions that never went
// through the autonomous refinery sidecar.
export function HandoffDigestSection({ sessionId, enabled }: HandoffDigestSectionProps) {
  const [open, setOpen] = useState(false)
  const { data, live } = useSessionHandoffDigest(sessionId, enabled)

  const digest = live ?? data
  if (!digest) return null

  return (
    <div className="rounded border border-border">
      <div className="flex items-center gap-2 px-3 py-2">
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 p-0 shrink-0"
          onClick={() => setOpen((prev) => !prev)}
          aria-label={open ? 'Collapse handoff digest' : 'Expand handoff digest'}
        >
          {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        </Button>
        <FileStack className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        <span className="text-xs font-medium flex-1">Handoff digest</span>
        <span className="text-xs text-muted-foreground">{digest.fold_count} folds</span>
        <span className="text-xs text-muted-foreground">·</span>
        <span className="text-xs text-muted-foreground">{formatUpdatedAt(digest.updated_at)}</span>
      </div>
      {open && (
        <div className="border-t border-border px-3 py-2 text-xs space-y-2">
          <RenderedMarkdown content={digest.content} />
        </div>
      )}
    </div>
  )
}
