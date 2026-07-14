import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'
import { Textarea } from '@/components/ui/Textarea'
import { Spinner } from '@/components/ui/Spinner'
import { useConsoleChat, useSendConsoleChatMessage, useCloseConsoleChat } from '@/hooks/useConsoleChats'
import { useConsoleChatStream } from '@/hooks/useConsoleChatStream'
import { TurnActiveError } from '@/api/consoleChats'
import { ChatMessageList } from './ChatMessageList'

interface ChatViewProps {
  sid: string
  onClosed: () => void
}

// Transcript + composer + header. Composer disables while turn==='running'
// (the BE 409s a second message) and shows a streaming indicator. Auto-scroll
// on new items.
export function ChatView({ sid, onClosed }: ChatViewProps) {
  const { data: detail } = useConsoleChat(sid)
  const stream = useConsoleChatStream(sid)
  const sendMutation = useSendConsoleChatMessage()
  const closeMutation = useCloseConsoleChat()
  const [text, setText] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)

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
          {stream.contextLeft != null && (
            <span className="text-xs text-muted-foreground">Context left: {stream.contextLeft}%</span>
          )}
          <Button variant="outline" size="sm" onClick={handleClose} disabled={closeMutation.isPending}>
            Close
          </Button>
        </div>
      </div>

      <div ref={scrollRef} className="flex-1 overflow-y-auto px-4 py-3">
        {stream.isLoadingHistory ? (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        ) : (
          <ChatMessageList
            sid={sid}
            transcript={stream.transcript}
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
          <Button onClick={handleSend} disabled={isRunning || !text.trim() || sendMutation.isPending}>
            {isRunning ? <Spinner size="sm" /> : 'Send'}
          </Button>
        </div>
      </div>
    </div>
  )
}
