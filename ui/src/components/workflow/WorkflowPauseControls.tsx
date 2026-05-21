import { useState } from 'react'
import { PauseCircle, XCircle } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Textarea } from '@/components/ui/Textarea'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'

interface WorkflowPauseControlsProps {
  onContinue: (instructions: string) => void
  continuePending?: boolean
  onFail?: (reason: string) => void
  failPending?: boolean
}

export function WorkflowPauseControls({ onContinue, continuePending, onFail, failPending }: WorkflowPauseControlsProps) {
  const [instructions, setInstructions] = useState('')
  const [failReason, setFailReason] = useState('')
  const [failConfirmOpen, setFailConfirmOpen] = useState(false)

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-yellow-200 bg-yellow-50 px-4 py-3 text-sm dark:border-yellow-800 dark:bg-yellow-950/30">
      <div className="flex items-center gap-2 text-yellow-700 dark:text-yellow-400">
        <PauseCircle className="h-4 w-4" />
        <Badge className="bg-yellow-500/20 text-yellow-700 dark:text-yellow-400 border-yellow-500/30">
          Waiting
        </Badge>
        <span className="font-medium">Workflow paused — awaiting operator input</span>
      </div>
      <div className="space-y-2">
        <Textarea
          value={instructions}
          onChange={(e) => setInstructions(e.target.value)}
          placeholder="Optional instructions for resumed workflow..."
          rows={2}
          className="text-sm"
        />
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            onClick={() => onContinue(instructions)}
            disabled={continuePending}
          >
            {continuePending ? <Spinner size="sm" className="mr-2" /> : null}
            Continue
          </Button>
          {onFail && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setFailConfirmOpen(true)}
              disabled={failPending || !failReason.trim()}
              className="text-destructive hover:text-destructive"
            >
              Fail Workflow
            </Button>
          )}
        </div>
        {onFail && (
          <Input
            value={failReason}
            onChange={(e) => setFailReason(e.target.value)}
            placeholder="Reason to fail (required to enable Fail button)"
            className="text-sm"
          />
        )}
      </div>
      {onFail && (
        <ConfirmDialog
          open={failConfirmOpen}
          onClose={() => setFailConfirmOpen(false)}
          onConfirm={() => {
            onFail(failReason.trim())
            setFailReason('')
          }}
          title="Fail Workflow"
          message={`Fail this workflow with reason: "${failReason}"?`}
          confirmLabel="Fail"
          variant="destructive"
        />
      )}
    </div>
  )
}

interface WorkflowFailControlProps {
  onFail: (reason: string) => void
  failPending?: boolean
}

export function WorkflowFailControl({ onFail, failPending }: WorkflowFailControlProps) {
  const [reason, setReason] = useState('')
  const [confirmOpen, setConfirmOpen] = useState(false)

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setConfirmOpen(true)}
        disabled={failPending || !reason.trim()}
        className="text-destructive hover:text-destructive"
      >
        {failPending ? (
          <Spinner size="sm" className="mr-2" />
        ) : (
          <XCircle className="h-4 w-4 mr-2" />
        )}
        Fail
      </Button>
      <Input
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        placeholder="Fail reason..."
        className="h-8 text-sm w-40"
      />
      <ConfirmDialog
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        onConfirm={() => {
          onFail(reason.trim())
          setReason('')
        }}
        title="Fail Workflow"
        message={`Fail the running workflow with reason: "${reason}"?`}
        confirmLabel="Fail"
        variant="destructive"
      />
    </>
  )
}
