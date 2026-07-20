import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { ChatSessionList } from '@/components/console/ChatSessionList'
import { NewChatForm } from '@/components/console/NewChatForm'
import { ChatView } from '@/components/console/ChatView'

// Left = session list + New chat form, right = the selected session's chat
// view. Selected session id lives in the URL (?session=…) so a reload
// restores it.
export function ConsolePage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const sid = searchParams.get('session') ?? undefined
  const [showNewChat, setShowNewChat] = useState(false)

  const selectSession = (id: string) => {
    setShowNewChat(false)
    setSearchParams({ session: id })
  }

  return (
    <div className="flex h-[calc(100vh-4rem)]">
      <div className="w-80 shrink-0 overflow-y-auto border-r border-border">
        <div className="flex items-center justify-between border-b border-border p-3">
          <h2 className="text-sm font-semibold">Console chats</h2>
          <Button size="sm" variant="outline" onClick={() => setShowNewChat((v) => !v)}>
            <Plus className="h-3.5 w-3.5" />
          </Button>
        </div>
        {showNewChat && <NewChatForm onCreated={selectSession} />}
        <ChatSessionList selectedSid={sid} onSelect={selectSession} />
      </div>
      <div className="flex-1 overflow-hidden">
        {sid ? (
          <ChatView
            key={sid}
            sid={sid}
            onClosed={() => setSearchParams({})}
            onDetach={() => setSearchParams({})}
            onOpenSibling={selectSession}
          />
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            Select a chat or start a new one.
          </div>
        )}
      </div>
    </div>
  )
}
