import { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { Spinner } from '@/components/ui/Spinner'
import {
  useConsoleChat,
  useSendConsoleChatMessage,
  useCloseConsoleChat,
  useInterruptConsoleChat,
} from '@/hooks/useConsoleChats'
import { useConsoleChatStream } from '@/hooks/useConsoleChatStream'
import { TurnActiveError } from '@/api/consoleChats'
import { ChatMessageList } from './ChatMessageList'

const XTerminal = lazy(() =>
  import('@/components/workflow/XTerminal').then((m) => ({ default: m.XTerminal }))
)

interface ChatViewProps {
  sid: string
  onClosed: () => void
  onDetach: () => void
}

// Transcript + composer + header. Composer disables while turn==='running'
// (the BE 409s a second message) and swaps Send for Stop (POST /interrupt —
// cancels the turn, keeps the engine alive). Detach deselects the chat and
// leaves the engine running for a later resume; Close tears it down.
// Auto-scroll on new items.
export function ChatView({ sid, onClosed, onDetach }: ChatViewProps) {
  const { data: detail } = useConsoleChat(sid)
  const stream = useConsoleChatStream(sid)
  const sendMutation = useSendConsoleChatMessage()
  const closeMutation = useCloseConsoleChat()
  const interruptMutation = useInterruptConsoleChat()
  const [text, setText] = useState('')
  const [showTerminal, setShowTerminal] = useState(false)
  const [search, setSearch] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)

  // Client-side transcript search: a non-empty query filters the merged
  // transcript by substring (case-insensitive) — mirrors the TUI's Ctrl+F.
  const searchActive = search.trim().length > 0
  const visibleTranscript = useMemo(() => {
    if (!searchActive) return stream.transcript
    const q = search.trim().toLowerCase()
    return stream.transcript.filter((item) =>
      item.kind === 'message'
        ? (item.message.content ?? '').toLowerCase().includes(q)
        : item.text.toLowerCase().includes(q)
    )
  }, [stream.transcript, search, searchActive])

  // Raw-terminal attach exists only for the claude engine (PTY-backed); the
  // relay is a viewer — closing the panel detaches without touching the chat.
  const canAttachTerminal = detail?.engine === 'claude' && detail?.live === true

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [stream.transcript.length])

  const isRunning = stream.turn === 'running'

  const handleSend = async () => {
    const value = text.trim()
    if (!value || isRunning) return
    setText('')
    try {
      await sendMutation.mutateAsync({ sid, text: value })
    } catch (e) {
      if (e instanceof TurnActiveError) {
        toast.error('A turn is already running.')
      } else {
        toast.error('Failed to send message.')
      }
    }
  }

  const handleClose = async () => {
    await closeMutation.mutateAsync(sid)
    onClosed()
  }

  const handleInterrupt = async () => {
    try {
      await interruptMutation.mutateAsync(sid)
    } catch {
      toast.error('Failed to interrupt the turn.')
    }
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <div className="min-w-0">
          <div className="text-sm font-semibold">
            {detail?.engine}
            {detail?.model && <span className="font-normal text-muted-foreground"> · {detail.model}</span>}
          </div>
          {stream.workDir && <div className="truncate text-xs text-muted-foreground">{stream.workDir}</div>}
        </div>
        <div className="flex items-center gap-3 shrink-0">
          <div className="flex items-center gap-1.5">
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Escape') setSearch('')
              }}
              placeholder="Search transcript…"
              className="h-8 w-44 text-xs"
              aria-label="Search transcript"
            />
            {searchActive && (
              <span className="text-xs text-muted-foreground whitespace-nowrap">
                {visibleTranscript.length} match{visibleTranscript.length === 1 ? '' : 'es'}
              </span>
            )}
          </div>
          {stream.contextLeft != null && (
            <span className="text-xs text-muted-foreground">Context left: {stream.contextLeft}%</span>
          )}
          {canAttachTerminal && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowTerminal((v) => !v)}
              title="Attach a raw terminal to the underlying claude CLI"
            >
              {showTerminal ? 'Hide terminal' : 'Terminal'}
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={onDetach}
            title="Leave the chat running and return to the list"
          >
            Detach
          </Button>
          <Button variant="outline" size="sm" onClick={handleClose} disabled={closeMutation.isPending}>
            Close
          </Button>
        </div>
      </div>

      {showTerminal && canAttachTerminal && (
        <div className="h-80 shrink-0 border-b border-border bg-black">
          <Suspense fallback={<div className="p-3 text-xs text-muted-foreground">Loading terminal…</div>}>
            <XTerminal sessionId={sid} onExit={() => setShowTerminal(false)} />
          </Suspense>
        </div>
      )}

      <div ref={scrollRef} className="flex-1 overflow-y-auto px-4 py-3">
        {stream.isLoadingHistory ? (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        ) : (
          <ChatMessageList
            sid={sid}
            transcript={visibleTranscript}
            approvals={stream.approvals}
            resolvedApprovals={stream.resolvedApprovals}
            liveThinking={stream.thinking}
            turn={stream.turn}
          />
        )}
      </div>

      <div className="border-t border-border p-3">
        <div className="flex items-end gap-2">
          <Textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                handleSend()
              }
            }}
            placeholder={isRunning ? 'Waiting for the agent to finish its turn…' : 'Message the agent…'}
            disabled={isRunning}
            className="min-h-[44px]"
          />
          {isRunning ? (
            <Button variant="destructive" onClick={handleInterrupt} disabled={interruptMutation.isPending}>
              {interruptMutation.isPending ? <Spinner size="sm" /> : 'Stop'}
            </Button>
          ) : (
            <Button onClick={handleSend} disabled={!text.trim() || sendMutation.isPending}>
              Send
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
