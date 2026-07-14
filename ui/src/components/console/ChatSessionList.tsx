import { useConsoleChats, useCloseConsoleChat } from '@/hooks/useConsoleChats'
import { formatTime } from '@/components/workflow/MessageTable'
import { Button } from '@/components/ui/Button'
import { cn } from '@/lib/utils'
import type { ConsoleChatSummary } from '@/types/consoleChat'

interface ChatSessionListProps {
  selectedSid: string | undefined
  onSelect: (sid: string) => void
}

// Rows: engine badge, model, project, status, started, a live dot; clicking a
// live session resumes it, a dead-but-user_interactive row is disabled/closable
// (the BE returns live=false for a session whose engine died with a
// hard-killed server).
export function ChatSessionList({ selectedSid, onSelect }: ChatSessionListProps) {
  const { data: chats = [], isLoading } = useConsoleChats()
  const closeMutation = useCloseConsoleChat()

  if (isLoading) {
    return <div className="p-3 text-sm text-muted-foreground">Loading…</div>
  }
  if (chats.length === 0) {
    return <div className="p-3 text-sm text-muted-foreground">No console chats yet.</div>
  }

  return (
    <div className="flex flex-col divide-y divide-border">
      {chats.map((chat) => (
        <ChatSessionRow
          key={chat.session_id}
          chat={chat}
          selected={chat.session_id === selectedSid}
          onSelect={() => onSelect(chat.session_id)}
          onClose={() => closeMutation.mutate(chat.session_id)}
        />
      ))}
    </div>
  )
}

function ChatSessionRow({
  chat,
  selected,
  onSelect,
  onClose,
}: {
  chat: ConsoleChatSummary
  selected: boolean
  onSelect: () => void
  onClose: () => void
}) {
  const isDeadInteractive = !chat.live && chat.status === 'user_interactive'

  return (
    <div
      className={cn(
        'flex items-center justify-between gap-2 px-3 py-2 text-sm',
        selected && 'bg-muted',
        isDeadInteractive ? 'opacity-60' : 'cursor-pointer hover:bg-muted/50'
      )}
      onClick={isDeadInteractive ? undefined : onSelect}
      data-testid="console-chat-row"
    >
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <span
            className={cn('h-1.5 w-1.5 rounded-full shrink-0', chat.live ? 'bg-emerald-500' : 'bg-muted-foreground/40')}
            aria-hidden
          />
          <span className="font-medium truncate">{chat.engine}</span>
          {chat.model && <span className="text-xs text-muted-foreground truncate">{chat.model}</span>}
        </div>
        <div className="truncate text-xs text-muted-foreground">
          {chat.project_id} · {chat.status} · {formatTime(chat.started_at)}
        </div>
      </div>
      {isDeadInteractive && (
        <Button
          size="sm"
          variant="outline"
          onClick={(e) => {
            e.stopPropagation()
            onClose()
          }}
        >
          Close
        </Button>
      )}
    </div>
  )
}
