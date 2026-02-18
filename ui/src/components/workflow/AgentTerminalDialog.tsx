import { lazy, Suspense } from 'react'
import { Terminal } from 'lucide-react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'

const XTerminal = lazy(() =>
  import('./XTerminal').then((m) => ({ default: m.XTerminal }))
)

interface AgentTerminalDialogProps {
  open: boolean
  onClose: () => void
  onExitSession: () => void
  exitPending: boolean
  sessionId: string
  agentType: string
}

export function AgentTerminalDialog({
  open,
  onClose,
  onExitSession,
  exitPending,
  sessionId,
  agentType,
}: AgentTerminalDialogProps) {
  // Ignore backdrop clicks — user must use close button or Exit Session
  const handleDialogClose = () => {
    // no-op: prevent backdrop dismiss
  }

  return (
    <Dialog open={open} onClose={handleDialogClose} className="max-w-6xl h-[80vh] flex flex-col">
      <DialogHeader onClose={onClose}>
        <div className="flex items-center gap-2">
          <Terminal className="h-5 w-5 text-blue-500" />
          <span className="font-semibold">Interactive Control</span>
          <span className="text-muted-foreground">— {agentType}</span>
        </div>
      </DialogHeader>
      <DialogBody className="flex-1 p-0 overflow-hidden">
        <Suspense
          fallback={
            <div className="flex items-center justify-center h-full">
              <Spinner size="lg" />
            </div>
          }
        >
          {open && (
            <XTerminal
              sessionId={sessionId}
              onExit={onClose}
            />
          )}
        </Suspense>
      </DialogBody>
      <DialogFooter>
        <Button
          variant="outline"
          size="sm"
          onClick={onExitSession}
          disabled={exitPending}
        >
          {exitPending && <Spinner size="sm" className="mr-2" />}
          Exit Session
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
