import { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import {
  useConsoleChat,
  useSendConsoleChatMessage,
  useCloseConsoleChat,
  useInterruptConsoleChat,
  useRevokeSessionApproval,
  useConsoleCatalog,
} from '@/hooks/useConsoleChats'
import { useConsoleChatStream } from '@/hooks/useConsoleChatStream'
import { TurnActiveError } from '@/api/consoleChats'
import { ChatMessageList } from './ChatMessageList'
import { ChatSiblingActions, isT0Profile } from './ChatSiblingActions'
import { ChatComposer } from './ChatComposer'
import { ChatStatusBar } from './ChatStatusBar'

const XTerminal = lazy(() =>
  import('@/components/workflow/XTerminal').then((m) => ({ default: m.XTerminal }))
)

interface ChatViewProps {
  sid: string
  onClosed: () => void
  onDetach: () => void
  // Called with a newly-spawned sibling's session id — both the direct
  // switch-model/hands-sibling mutation responses and the WS
  // console_chat.sibling_opened event (for other tabs watching this
  // session) drive it; ConsolePage passes selectSession.
  onOpenSibling: (sid: string) => void
}

// Transcript + composer + header. Composer disables while turn==='running'
// (the BE 409s a second message) and swaps Send for Stop (POST /interrupt —
// cancels the turn, keeps the engine alive). Detach deselects the chat and
// leaves the engine running for a later resume; Close tears it down.
// Auto-scroll on new items.
export function ChatView({ sid, onClosed, onDetach, onOpenSibling }: ChatViewProps) {
  const { data: detail } = useConsoleChat(sid)
  const { data: catalog } = useConsoleCatalog()
  const profileDisplayName = catalog?.profiles.find((p) => p.name === detail?.profile)?.display_name
  const stream = useConsoleChatStream(sid)
  const sendMutation = useSendConsoleChatMessage()
  const closeMutation = useCloseConsoleChat()
  const interruptMutation = useInterruptConsoleChat()
  const revokeMutation = useRevokeSessionApproval()
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

  // Direct switch-model/hands-sibling responses already call onOpenSibling;
  // this covers the WS event for other tabs watching this session — an
  // idempotent re-select when it's this tab that triggered the mutation.
  useEffect(() => {
    if (stream.siblingOpened) onOpenSibling(stream.siblingOpened.sibling_session_id)
  }, [stream.siblingOpened, onOpenSibling])

  const isRunning = stream.turn === 'running'

  const handleSend = async (value: string) => {
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

  const handleRevoke = async (tool: string) => {
    try {
      await revokeMutation.mutateAsync({ sid, tool })
    } catch {
      toast.error(`Failed to revoke ${tool}.`)
    }
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-end border-b border-border px-4 py-3">
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
          {detail && isT0Profile(detail.profile) && (
            <ChatSiblingActions
              sid={sid}
              engine={detail.engine}
              model={detail.model}
              onOpenSibling={onOpenSibling}
            />
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

      {stream.sessionApprovals.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5 border-b border-border px-4 py-1.5">
          <span className="text-xs text-muted-foreground">Always allowed:</span>
          {stream.sessionApprovals.map((tool) => (
            <span
              key={tool}
              className="inline-flex items-center gap-1 rounded-full border border-border bg-muted px-2 py-0.5 text-xs"
            >
              {tool}
              <Button
                variant="ghost"
                size="sm"
                className="h-4 w-4 p-0 text-muted-foreground hover:text-foreground"
                onClick={() => handleRevoke(tool)}
                disabled={revokeMutation.isPending}
                aria-label={`Revoke ${tool}`}
                title="Ask again before the next use"
              >
                ×
              </Button>
            </span>
          ))}
        </div>
      )}

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
            rotations={stream.rotations}
          />
        )}
      </div>

      <ChatComposer
        sid={sid}
        isRunning={isRunning}
        sendPending={sendMutation.isPending}
        stopPending={interruptMutation.isPending}
        onSend={handleSend}
        onStop={handleInterrupt}
      />
      <ChatStatusBar
        engine={detail?.engine}
        model={detail?.model}
        profile={profileDisplayName}
        workDir={stream.workDir}
        contextLeft={stream.contextLeft}
        cost={stream.cost}
        turn={stream.turn}
      />
    </div>
  )
}
