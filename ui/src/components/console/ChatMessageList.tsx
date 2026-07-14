import { RenderedMarkdown } from '@/components/ui/RenderedMarkdown'
import { formatTime } from '@/components/workflow/MessageTable'
import { ChatThinking } from './ChatThinking'
import { ChatToolCard } from './ChatToolCard'
import { ApprovalCard } from './ApprovalCard'
import { pairToolMessages, type MergedTranscriptItem, type ResolvedApproval } from './chatStream'
import type { PendingApproval } from '@/types/consoleChat'
import type { MessageWithTime } from '@/types/workflow'

interface ChatMessageListProps {
  sid: string
  transcript: MergedTranscriptItem[]
  approvals: PendingApproval[]
  resolvedApprovals: Map<string, ResolvedApproval>
  liveThinking: string[]
  turn: 'idle' | 'running'
}

// Renders the merged item list: user_input as user bubbles, text as assistant
// markdown, the in-flight delta buffer as a streaming bubble, thinking via
// ChatThinking, tool pairs via ChatToolCard, error rows in destructive
// styling — mirroring MessageTable's category styling (not refactoring it).
export function ChatMessageList({ sid, transcript, approvals, resolvedApprovals, liveThinking, turn }: ChatMessageListProps) {
  // mergeStream orders persisted rows first, live deltas appended after — so
  // the two kinds can be split and rendered as two passes without tracking a
  // running index across a single mixed iteration.
  const messageItems = transcript.filter(
    (item): item is Extract<MergedTranscriptItem, { kind: 'message' }> => item.kind === 'message'
  )
  const liveItems = transcript.filter(
    (item): item is Extract<MergedTranscriptItem, { kind: 'live' }> => item.kind === 'live'
  )

  const pairs = pairToolMessages(messageItems.map((item) => item.message))
  const pairByInvokeIndex = new Map(pairs.map((p) => [p.invokeIndex, p]))
  const consumedResultIndices = new Set(pairs.filter((p) => p.resultIndex !== undefined).map((p) => p.resultIndex!))

  return (
    <div className="flex flex-col gap-2">
      {messageItems.map((item, idx) => {
        if (consumedResultIndices.has(idx)) return null
        const pair = pairByInvokeIndex.get(idx)
        if (pair) return <ChatToolCard key={idx} pair={pair} />
        return <PersistedMessageRow key={idx} message={item.message} />
      })}

      {liveItems.map((item) => (
        <div key={`live-${item.itemId}`} className="rounded-md border border-border bg-background px-3 py-2">
          <RenderedMarkdown content={item.text} />
        </div>
      ))}

      {turn === 'running' && liveThinking.length > 0 && <ChatThinking text={liveThinking.join('')} />}

      {approvals.map((a) => (
        <ApprovalCard key={a.approval_id} sid={sid} approval={a} resolved={resolvedApprovals.get(a.approval_id)} />
      ))}
    </div>
  )
}

function PersistedMessageRow({ message }: { message: MessageWithTime }) {
  if (message.category === 'user_input') {
    return (
      <div className="rounded-md border-l-4 border-l-primary bg-primary/5 px-3 py-2 text-sm">
        <div className="text-[10px] text-muted-foreground mb-1">{formatTime(message.created_at)}</div>
        <div className="whitespace-pre-wrap break-words">{message.content}</div>
      </div>
    )
  }
  if (message.category === 'thinking') {
    return <ChatThinking text={message.content} />
  }
  if (message.category === 'error') {
    return (
      <div className="rounded-md border-l-4 border-l-destructive bg-destructive/5 px-3 py-2 text-sm text-destructive whitespace-pre-wrap break-words">
        {message.content}
      </div>
    )
  }
  if (message.category === 'text') {
    return (
      <div className="rounded-md border border-border bg-background px-3 py-2">
        <RenderedMarkdown content={message.content} />
      </div>
    )
  }
  // Unpaired tool/skill/subagent/result/validation rows.
  return (
    <div className="rounded-md border border-border bg-muted/20 px-3 py-2 text-xs whitespace-pre-wrap break-words font-mono">
      {message.content}
    </div>
  )
}
