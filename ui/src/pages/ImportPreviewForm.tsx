import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { Dropdown } from '@/components/ui/Dropdown'
import { Spinner } from '@/components/ui/Spinner'
import { listWorkflowDefs } from '@/api/workflows'
import { commitImport, type ImportPreviewResponse, type AttachedRef } from '@/api/specImport'
import { useProjectStore } from '@/stores/projectStore'

interface ImportPreviewFormProps {
  instanceId: string
  preview: ImportPreviewResponse
  onCommitted: (ticketId: string) => void
}

export function ImportPreviewForm({ instanceId, preview, onCommitted }: ImportPreviewFormProps) {
  const navigate = useNavigate()
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  const [title, setTitle] = useState(preview.title)
  const [description, setDescription] = useState(preview.description)
  const [instructions, setInstructions] = useState(preview.instructions)
  const [workflowName, setWorkflowName] = useState('')
  const [refs, setRefs] = useState<AttachedRef[]>(preview.attached_refs ?? [])
  const [submitting, setSubmitting] = useState(false)

  const { data: workflowDefs, isLoading: defsLoading } = useQuery({
    queryKey: ['workflows', 'defs', project],
    queryFn: listWorkflowDefs,
    enabled: projectsLoaded,
  })

  const workflowOptions = workflowDefs
    ? Object.keys(workflowDefs).map((id) => ({ value: id, label: id }))
    : []

  // Set default workflow once defs load
  useEffect(() => {
    if (!workflowDefs || workflowName) return
    const ids = Object.keys(workflowDefs)
    if (ids.length === 0) return
    const suggested = preview.suggested_workflow && ids.includes(preview.suggested_workflow)
      ? preview.suggested_workflow
      : ids[0]
    setWorkflowName(suggested)
  }, [workflowDefs, workflowName, preview.suggested_workflow])

  function removeRef(index: number) {
    setRefs((prev) => prev.filter((_, i) => i !== index))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!workflowName) return
    setSubmitting(true)
    try {
      const result = await commitImport(instanceId, {
        title,
        description,
        workflow_name: workflowName,
        instructions,
      })
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
          rows={4}
          className="resize-y"
        />
      </div>

      <div>
        <label className="text-sm font-medium mb-1 block">Agent Instructions</label>
        <Textarea
          value={instructions}
          onChange={(e) => setInstructions(e.target.value)}
          rows={5}
          className="resize-y"
        />
      </div>

      <div>
        <label className="text-sm font-medium mb-1 block">Workflow</label>
        {defsLoading ? (
          <Spinner size="sm" />
        ) : (
          <Dropdown
            value={workflowName}
            onChange={setWorkflowName}
            options={workflowOptions}
            placeholder="Select workflow…"
          />
        )}
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
        <Button type="submit" disabled={submitting || !workflowName}>
          {submitting ? <Spinner size="sm" className="mr-2" /> : null}
          Create Ticket
        </Button>
      </div>
    </form>
  )
}
