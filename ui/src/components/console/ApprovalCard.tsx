import { Button } from '@/components/ui/Button'
import { useReplyApproval } from '@/hooks/useConsoleChats'
import type { PendingApproval } from '@/types/consoleChat'
import type { ResolvedApproval } from './chatStream'
import { parseQuestions, QuestionCard } from './QuestionCard'

interface ApprovalCardProps {
  sid: string
  approval: PendingApproval
  resolved?: ResolvedApproval
}

// Inline card: kind + command/patch preview + cwd + reason, Allow /
// Always allow (allow_for_session — the engine remembers the tool for the
// rest of the chat) / Deny buttons, and terminal states driven by
// console_chat.approval_resolved — 'Allowed' / 'Denied' / 'Denied — timed
// out'. The BE emits a resolved push with decision+reason for the
// timeout/engine-stop paths too, so this must never be left spinning once
// resolved is set.
export function ApprovalCard({ sid, approval, resolved }: ApprovalCardProps) {
  const replyMutation = useReplyApproval()

  // AskUserQuestion renders as an interactive question card; an unparseable
  // payload falls through to the generic card (its Allow maps to the
  // server-side plain-text redirect, never the unreachable TUI picker).
  const questions = approval.tool === 'AskUserQuestion' ? parseQuestions(approval.input) : null
  if (questions) {
    return <QuestionCard sid={sid} approval={approval} questions={questions} resolved={resolved} />
  }

  const isTimedOut = resolved?.decision === 'deny' && !!resolved.reason?.toLowerCase().includes('timed out')
  const statusLabel = resolved ? (resolved.decision !== 'deny' ? 'Allowed' : isTimedOut ? 'Denied — timed out' : 'Denied') : null

  return (
    <div
      className="rounded-md border border-amber-400/50 bg-amber-50/50 dark:bg-amber-950/20 px-3 py-2 text-xs"
      data-testid="approval-card"
    >
      <div className="font-semibold text-foreground">{approval.kind} approval requested</div>
      <pre className="mt-1 whitespace-pre-wrap break-words font-mono text-foreground/90">{approval.command}</pre>
      <div className="mt-1 text-muted-foreground">cwd: {approval.cwd}</div>
      {approval.reason && <div className="mt-1 text-muted-foreground">{approval.reason}</div>}

      {statusLabel ? (
        <div className="mt-2 text-xs font-semibold text-foreground">{statusLabel}</div>
      ) : (
        <div className="mt-2 flex gap-2">
          <Button
            size="sm"
            variant="default"
            disabled={replyMutation.isPending}
            onClick={() => replyMutation.mutate({ sid, aid: approval.approval_id, decision: 'allow' })}
          >
            Allow
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={replyMutation.isPending}
            title="Allow this tool for the rest of the chat"
            onClick={() => replyMutation.mutate({ sid, aid: approval.approval_id, decision: 'allow_for_session' })}
          >
            Always allow
          </Button>
          <Button
            size="sm"
            variant="destructive"
            disabled={replyMutation.isPending}
            onClick={() => replyMutation.mutate({ sid, aid: approval.approval_id, decision: 'deny' })}
          >
            Deny
          </Button>
        </div>
      )}
    </div>
  )
}
