import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { X } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { Spinner } from '@/components/ui/Spinner'
import { commitImport, type ImportPreviewResponse, type AttachedRef } from '@/api/specImport'

interface ImportPreviewFormProps {
  instanceId: string
  preview: ImportPreviewResponse
  onCommitted: (ticketId: string) => void
}

export function ImportPreviewForm({ instanceId, preview, onCommitted }: ImportPreviewFormProps) {
  const navigate = useNavigate()

  const [title, setTitle] = useState(preview.title)
  const [description, setDescription] = useState(preview.description)
  const [refs, setRefs] = useState<AttachedRef[]>(preview.attached_refs ?? [])
  const [submitting, setSubmitting] = useState(false)

  function removeRef(index: number) {
    setRefs((prev) => prev.filter((_, i) => i !== index))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    try {
      const result = await commitImport(instanceId, { title, description })
      toast.success('Ticket created')
      onCommitted(result.ticket_id)
      navigate(`/tickets/${result.ticket_id}`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create ticket')
      setSubmitting(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div>
        <label className="text-sm font-medium mb-1 block">Title</label>
        <Input value={title} onChange={(e) => setTitle(e.target.value)} required />
      </div>

      <div>
        <label className="text-sm font-medium mb-1 block">Description</label>
        <Textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          rows={8}
          className="resize-y"
        />
      </div>

      {refs.length > 0 && (
        <div>
          <label className="text-sm font-medium mb-1 block">Attached References</label>
          <div className="flex flex-wrap gap-2">
            {refs.map((ref, i) => (
              <div
                key={i}
                className="inline-flex items-center gap-1 rounded-full border border-border bg-muted px-3 py-1 text-xs"
              >
                <a
                  href={ref.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="hover:underline"
                >
                  {ref.label ?? ref.url}
                </a>
                <button
                  type="button"
                  onClick={() => removeRef(i)}
                  className="ml-1 text-muted-foreground hover:text-foreground"
                  aria-label="Remove reference"
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="flex justify-end pt-2">
        <Button type="submit" disabled={submitting}>
          {submitting ? <Spinner size="sm" className="mr-2" /> : null}
          Create Ticket
        </Button>
      </div>
    </form>
  )
}
