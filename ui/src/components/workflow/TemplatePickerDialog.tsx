import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { Spinner } from '@/components/ui/Spinner'
import { listDefaultTemplates, type DefaultTemplate } from '@/api/defaultTemplates'

interface TemplatePickerDialogProps {
  open: boolean
  onClose: () => void
  onApply: (template: string) => void
  hasExistingPrompt: boolean
}

export function TemplatePickerDialog({ open, onClose, onApply, hasExistingPrompt }: TemplatePickerDialogProps) {
  const [selectedId, setSelectedId] = useState('')

  const { data: templates = [], isLoading } = useQuery({
    queryKey: ['default-templates', 'list'],
    queryFn: listDefaultTemplates,
    enabled: open,
  })

  const selectedTemplate = templates.find((t: DefaultTemplate) => t.id === selectedId)

  const handleApply = () => {
    if (!selectedTemplate) return
    onApply(selectedTemplate.template)
    onClose()
  }

  const handleClose = () => {
    setSelectedId('')
    onClose()
  }

  const options = templates.map((t: DefaultTemplate) => ({ value: t.id, label: t.name }))

  return (
    <Dialog open={open} onClose={handleClose}>
      <DialogHeader onClose={handleClose}>
        <h3 className="text-lg font-semibold">Apply Default Template</h3>
      </DialogHeader>
      <DialogBody className="overflow-visible">
        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <Spinner />
          </div>
        ) : templates.length === 0 ? (
          <p className="text-sm text-muted-foreground py-4">No default templates available.</p>
        ) : (
          <div className="space-y-3">
            <div>
              <label className="block text-xs font-medium text-muted-foreground mb-1">Template</label>
              <Dropdown
                value={selectedId}
                onChange={setSelectedId}
                options={options}
                placeholder="Select a template..."
              />
            </div>
            {selectedTemplate && (
              <div>
                <label className="block text-xs font-medium text-muted-foreground mb-1">Preview</label>
                <div className="rounded-md border border-border bg-muted/30 p-3 max-h-64 overflow-y-auto">
                  <pre className="text-sm whitespace-pre-wrap break-words font-mono">{selectedTemplate.template}</pre>
                </div>
              </div>
            )}
            {hasExistingPrompt && selectedTemplate && (
              <div className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-200">
                The current agent prompt is not empty. Applying this template will replace it.
              </div>
            )}
          </div>
        )}
      </DialogBody>
      <DialogFooter>
        <Button variant="ghost" size="sm" onClick={handleClose}>
          Cancel
        </Button>
        <Button
          size="sm"
          variant={hasExistingPrompt ? 'destructive' : 'default'}
          onClick={handleApply}
          disabled={!selectedTemplate}
        >
          {hasExistingPrompt && selectedTemplate ? 'Replace Current Prompt' : 'Apply'}
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
