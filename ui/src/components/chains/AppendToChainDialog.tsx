import { useState, useEffect, useMemo } from 'react'
import { ListPlus } from 'lucide-react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { ChainTicketSelector } from './ChainTicketSelector'
import { useAppendToChain } from '@/hooks/useChains'
import type { ChainExecution } from '@/types/chain'

interface AppendToChainDialogProps {
  open: boolean
  onClose: () => void
  chain: ChainExecution
}

export function AppendToChainDialog({ open, onClose, chain }: AppendToChainDialogProps) {
  const [ticketIds, setTicketIds] = useState<string[]>([])
  const [epicIds, setEpicIds] = useState<string[]>([])

  const appendMutation = useAppendToChain()

  // Ticket IDs already in the chain — used to exclude from selector
  const existingTicketIds = useMemo(
    () => new Set(chain.items?.map((i) => i.ticket_id) ?? []),
    [chain.items]
  )

  // Reset on close
  useEffect(() => {
    if (!open) {
      setTicketIds([])
      setEpicIds([])
      appendMutation.reset()
    }
  }, [open]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleSubmit = async () => {
    if (ticketIds.length === 0) return
    // Exclude epic IDs — epics aren't chain items
    const epicSet = new Set(epicIds)
    const childOnlyIds = ticketIds.filter((id) => !epicSet.has(id))
    // Exclude tickets already in the chain
    const newIds = (childOnlyIds.length > 0 ? childOnlyIds : ticketIds).filter(
      (id) => !existingTicketIds.has(id)
    )
    if (newIds.length === 0) return
    try {
      await appendMutation.mutateAsync({ id: chain.id, data: { ticket_ids: newIds } })
      onClose()
    } catch {
      // Error handled by mutation state
    }
  }

  const canSubmit = ticketIds.length > 0 && !appendMutation.isPending

  return (
    <Dialog open={open} onClose={onClose}>
      <DialogHeader onClose={onClose}>
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <ListPlus className="h-5 w-5" />
          Append Tickets to Chain
        </h2>
      </DialogHeader>

      <DialogBody className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Select tickets to append to the running chain. Tickets already in the chain are excluded.
        </p>
        <ChainTicketSelector
          selectedIds={ticketIds}
          onChange={setTicketIds}
          onEpicIdsChange={setEpicIds}
          excludeIds={existingTicketIds}
        />

        {appendMutation.isError && (
          <p className="text-sm text-destructive">
            {appendMutation.error instanceof Error ? appendMutation.error.message : 'Append failed'}
          </p>
        )}
      </DialogBody>

      <DialogFooter>
        <Button variant="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={!canSubmit}>
          {appendMutation.isPending && <Spinner size="sm" className="mr-2" />}
          Append
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
