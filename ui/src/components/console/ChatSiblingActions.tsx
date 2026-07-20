import { useState } from 'react'
import { toast } from 'sonner'
import { Dropdown } from '@/components/ui/Dropdown'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { useConsoleCatalog, useSwitchConsoleChatModel, useOpenHandsSibling } from '@/hooks/useConsoleChats'

// Profiles whose chats get the sibling affordances below: 't0-decider' picks
// a model and spawns a sibling with it (the engine is never mutated in
// place); 'Open hands sibling' works for any t0 profile since it always
// targets the t0-hands companion.
const T0_PROFILES = new Set(['t0-decider', 't0-hands'])

export function isT0Profile(profile: string | undefined): boolean {
  return !!profile && T0_PROFILES.has(profile)
}

interface ChatSiblingActionsProps {
  sid: string
  engine: string
  model?: string
  onOpenSibling: (sid: string) => void
}

// Header affordances for a t0-profile chat: a compact model Dropdown +
// 'Switch model' (spawns a sibling on the same engine with the chosen
// model — model changes never mutate the running engine in place) and
// 'Open hands sibling' (spawns a t0-hands sibling pre-seeded with this
// chat's refinery digest). Both mutations return the new session id, which
// the caller uses to select it; the WS console_chat.sibling_opened event
// covers other tabs watching the same session.
export function ChatSiblingActions({ sid, engine, model, onOpenSibling }: ChatSiblingActionsProps) {
  const { data: catalog } = useConsoleCatalog()
  const switchModelMutation = useSwitchConsoleChatModel()
  const handsSiblingMutation = useOpenHandsSibling()
  const [selectedModel, setSelectedModel] = useState(model ?? '')

  const engineOption = catalog?.engines.find((e) => e.id === engine)
  const modelOptions = (engineOption?.models ?? []).map((m) => ({ value: m.id, label: m.display_name }))

  const handleSwitchModel = async () => {
    if (!selectedModel || selectedModel === model) return
    try {
      const resp = await switchModelMutation.mutateAsync({ sid, req: { model: selectedModel } })
      onOpenSibling(resp.sibling_session_id)
    } catch {
      toast.error('Failed to switch model.')
    }
  }

  const handleOpenHandsSibling = async () => {
    try {
      const resp = await handsSiblingMutation.mutateAsync(sid)
      onOpenSibling(resp.sibling_session_id)
    } catch {
      toast.error('Failed to open hands sibling.')
    }
  }

  return (
    <div className="flex items-center gap-1.5">
      <Dropdown
        value={selectedModel}
        onChange={setSelectedModel}
        options={modelOptions}
        placeholder="Model…"
        disabled={modelOptions.length === 0}
        className="h-8 w-36 text-xs"
      />
      <Button
        variant="outline"
        size="sm"
        onClick={handleSwitchModel}
        disabled={!selectedModel || selectedModel === model || switchModelMutation.isPending}
        title="Spawn a sibling chat with the selected model"
      >
        {switchModelMutation.isPending ? <Spinner size="sm" /> : 'Switch model'}
      </Button>
      <Button
        variant="outline"
        size="sm"
        onClick={handleOpenHandsSibling}
        disabled={handsSiblingMutation.isPending}
        title="Open a t0-hands sibling seeded with this chat's refinery digest"
      >
        {handsSiblingMutation.isPending ? <Spinner size="sm" /> : 'Open hands sibling'}
      </Button>
    </div>
  )
}
