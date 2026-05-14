import { Terminal } from 'lucide-react'
import { useInteractiveSessionsStore } from '@/stores/interactiveSessionsStore'

export function InteractiveSessionsTab() {
  const sessions = useInteractiveSessionsStore((s) => s.sessions)
  const toggleMinimized = useInteractiveSessionsStore((s) => s.toggleMinimized)

  if (sessions.length === 0) return null

  return (
    <button
      onClick={toggleMinimized}
      className="flex items-center p-2 rounded-md text-blue-500 hover:text-blue-600 hover:bg-muted transition-colors"
      title="Interactive Sessions"
    >
      <Terminal className="h-5 w-5" />
      <span className="hidden md:inline ml-1 text-xs">Sessions ({sessions.length})</span>
    </button>
  )
}
