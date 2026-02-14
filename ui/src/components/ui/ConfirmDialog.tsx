import { Dialog, DialogHeader, DialogBody, DialogFooter } from './Dialog'
import { Button } from './Button'

interface ConfirmDialogProps {
  open: boolean
  onClose: () => void
  onConfirm: () => void
  title: string
  message: string
  confirmLabel?: string
  variant?: 'default' | 'destructive'
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  message,
  confirmLabel = 'Confirm',
  variant = 'default',
}: ConfirmDialogProps) {
  return (
    <Dialog open={open} onClose={onClose} className="max-w-md">
      <DialogHeader onClose={onClose}>
        <h3 className="text-lg font-semibold">{title}</h3>
      </DialogHeader>
      <DialogBody>
        <p className="text-sm text-muted-foreground">{message}</p>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" size="sm" onClick={onClose}>
          Cancel
        </Button>
        <Button
          variant={variant}
          size="sm"
          onClick={() => {
            onConfirm()
            onClose()
          }}
        >
          {confirmLabel}
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
