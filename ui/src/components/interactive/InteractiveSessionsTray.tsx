import { useState, useEffect, useCallback } from 'react'
import { Terminal, ChevronDown, ChevronUp, X } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { useInteractiveSessionsStore } from '@/stores/interactiveSessionsStore'
import { useWebSocketContext } from '@/providers/WebSocketProvider'
import {
  useExitInteractive,
  useExitInteractiveProject,
  useKillInteractive,
  useKillInteractiveProject,
} from '@/hooks/useTickets'
import { InteractiveSessionPanel } from './InteractiveSessionPanel'
import type { WSEvent } from '@/hooks/useWebSocket'
import type { InteractiveSession } from '@/stores/interactiveSessionsStore'

export function InteractiveSessionsTray() {
  const sessions = useInteractiveSessionsStore((s) => s.sessions)
  const activeId = useInteractiveSessionsStore((s) => s.activeId)
  const minimized = useInteractiveSessionsStore((s) => s.minimized)
  const setActive = useInteractiveSessionsStore((s) => s.setActive)
  const toggleMinimized = useInteractiveSessionsStore((s) => s.toggleMinimized)
  const remove = useInteractiveSessionsStore((s) => s.remove)

  const [showKillConfirm, setShowKillConfirm] = useState(false)

  const exitTicketMutation = useExitInteractive()
  const exitProjectMutation = useExitInteractiveProject()
  const killTicketMutation = useKillInteractive()
  const killProjectMutation = useKillInteractiveProject()

  const { addEventListener, removeEventListener } = useWebSocketContext()

  const handleWSEvent = useCallback(
    (event: WSEvent) => {
      if (event.type === 'agent.killed' || event.type === 'agent.completed') {
        const sessionId = event.data?.session_id as string | undefined
        if (sessionId) remove(sessionId)
      }
    },
    [remove]
  )

  useEffect(() => {
    addEventListener(handleWSEvent)
    return () => removeEventListener(handleWSEvent)
  }, [addEventListener, removeEventListener, handleWSEvent])

  if (sessions.length === 0) return null

  const activeSession = sessions.find((s) => s.sessionId === activeId) ?? sessions[0]

  const handleExitSession = () => {
    if (!activeSession) return
    const params = {
      workflow: activeSession.workflow,
      session_id: activeSession.sessionId,
      instance_id: activeSession.instanceId,
    }
    if (activeSession.scope.type === 'ticket') {
      exitTicketMutation.mutate(
        { ticketId: activeSession.scope.ticketId, params },
        { onSuccess: () => remove(activeSession.sessionId) }
      )
    } else {
      exitProjectMutation.mutate(
        { projectId: activeSession.scope.projectId, params },
        { onSuccess: () => remove(activeSession.sessionId) }
      )
    }
  }

  const handleKillConfirmed = () => {
    if (!activeSession) return
    const params = {
      workflow: activeSession.workflow,
      session_id: activeSession.sessionId,
      instance_id: activeSession.instanceId,
    }
    if (activeSession.scope.type === 'ticket') {
      killTicketMutation.mutate(
        { ticketId: activeSession.scope.ticketId, params },
        { onSuccess: () => remove(activeSession.sessionId) }
      )
    } else {
      killProjectMutation.mutate(
        { projectId: activeSession.scope.projectId, params },
        { onSuccess: () => remove(activeSession.sessionId) }
      )
    }
  }

  const exitPending =
    exitTicketMutation.isPending || exitProjectMutation.isPending
  const killPending =
    killTicketMutation.isPending || killProjectMutation.isPending

  return (
    <div className="fixed bottom-0 left-0 right-0 z-40 border-t border-border bg-background flex flex-col">
      <TrayHeader
        sessions={sessions}
        activeId={activeId}
        minimized={minimized}
        onSetActive={setActive}
        onToggleMinimized={toggleMinimized}
        onClose={() => setShowKillConfirm(true)}
        onExitSession={handleExitSession}
        exitPending={exitPending}
        killPending={killPending}
        activeSession={activeSession}
      />

      <div className={minimized ? 'hidden' : 'flex flex-col h-[50vh]'}>
        {sessions.map((s) => (
          <InteractiveSessionPanel
            key={s.sessionId}
            sessionId={s.sessionId}
            isActive={s.sessionId === activeId}
            onExit={() => remove(s.sessionId)}
          />
        ))}
        {activeSession?.agentType === 'planner' && (
          <div className="px-4 py-2 text-sm text-muted-foreground border-t border-border">
            On exit, the plan file will be used as instructions for workflow agents. Use &apos;/plan&apos; to show the plan.
          </div>
        )}
      </div>

      <ConfirmDialog
        open={showKillConfirm}
        onClose={() => setShowKillConfirm(false)}
        onConfirm={handleKillConfirmed}
        title="Close Session"
        message="Force-close this interactive session? The workflow will not resume."
        confirmLabel="Close Session"
        variant="destructive"
      />
    </div>
  )
}

interface TrayHeaderProps {
  sessions: InteractiveSession[]
  activeId: string
  minimized: boolean
  onSetActive: (id: string) => void
  onToggleMinimized: () => void
  onClose: () => void
  onExitSession: () => void
  exitPending: boolean
  killPending: boolean
  activeSession: InteractiveSession | undefined
}

function TrayHeader({
  sessions,
  activeId,
  minimized,
  onSetActive,
  onToggleMinimized,
  onClose,
  onExitSession,
  exitPending,
  killPending,
  activeSession,
}: TrayHeaderProps) {
  return (
    <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border">
      <Terminal className="h-4 w-4 text-blue-500 shrink-0" />
      <div className="flex items-center gap-1 flex-1 overflow-x-auto">
        {sessions.map((s) => {
          const shortId = s.sessionId.slice(0, 6)
          const label = `${s.agentType} (${shortId})`
          const isActive = s.sessionId === activeId
          return (
            <button
              key={s.sessionId}
              onClick={() => onSetActive(s.sessionId)}
              className={`px-2 py-0.5 rounded text-xs whitespace-nowrap transition-colors ${
                isActive
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted'
              }`}
            >
              {label}
            </button>
          )
        })}
      </div>
      <div className="flex items-center gap-1 shrink-0">
        <Button
          variant="ghost"
          size="sm"
          onClick={onExitSession}
          disabled={exitPending}
          className="text-xs h-7 px-2"
        >
          {exitPending && <Spinner size="sm" className="mr-1" />}
          Exit Session
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={onClose}
          disabled={killPending}
          className="text-xs h-7 px-2 text-destructive hover:text-destructive"
        >
          {killPending ? <Spinner size="sm" /> : <X className="h-3.5 w-3.5" />}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={onToggleMinimized}
          className="h-7 w-7 p-0"
          title={minimized ? 'Expand' : 'Minimize'}
        >
          {minimized ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
        </Button>
      </div>
      {minimized && activeSession && (
        <span className="text-xs text-muted-foreground ml-2">
          Interactive Control — {activeSession.agentType}
        </span>
      )}
    </div>
  )
}
