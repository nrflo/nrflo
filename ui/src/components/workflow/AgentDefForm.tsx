import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { MarkdownEditor } from '@/components/ui/MarkdownEditor'
import type { AgentDef, AgentDefCreateRequest, AgentDefUpdateRequest } from '@/types/workflow'

export function AgentDefForm({
  initial,
  onSubmit,
  onCancel,
  isCreate,
}: {
  initial?: Partial<AgentDef>
  onSubmit: (data: AgentDefCreateRequest | AgentDefUpdateRequest) => void
  onCancel: () => void
  isCreate: boolean
}) {
  const [id, setId] = useState(initial?.id || '')
  const [model, setModel] = useState(initial?.model || 'sonnet')
  const [timeout, setTimeout] = useState(initial?.timeout || 20)
  const [restartThreshold, setRestartThreshold] = useState<number | ''>(initial?.restart_threshold ?? '')
  const [prompt, setPrompt] = useState(initial?.prompt || '')

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (isCreate && !prompt.trim()) return
    const threshold = restartThreshold !== '' ? restartThreshold : undefined
    if (isCreate) {
      onSubmit({ id, model, timeout, prompt, restart_threshold: threshold } as AgentDefCreateRequest)
    } else {
      onSubmit({ model, timeout, prompt, restart_threshold: threshold } as AgentDefUpdateRequest)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3 p-4 border border-border rounded-lg bg-muted/30">
      {isCreate && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">ID</label>
          <input
            type="text"
            value={id}
            onChange={(e) => setId(e.target.value)}
            placeholder="e.g., setup-analyzer"
            required
            className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
          />
        </div>
      )}
      <div className="flex gap-3">
        <div className="flex-1">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Model</label>
          <Dropdown
            value={model}
            onChange={setModel}
            options={[
              { value: 'opus', label: 'opus' },
              { value: 'sonnet', label: 'sonnet' },
              { value: 'haiku', label: 'haiku' },
              { value: 'gpt_5.3', label: 'gpt_5.3' },
            ]}
          />
        </div>
        <div className="w-32">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Timeout (min)</label>
          <input
            type="number"
            value={timeout}
            onChange={(e) => setTimeout(Number(e.target.value))}
            min={1}
            className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
          />
        </div>
        <div className="w-32">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Restart % (ctx)</label>
          <input
            type="number"
            value={restartThreshold}
            onChange={(e) => setRestartThreshold(e.target.value === '' ? '' : Number(e.target.value))}
            placeholder="25"
            min={1}
            max={99}
            className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
          />
        </div>
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Prompt Template</label>
        <MarkdownEditor
          value={prompt}
          onChange={setPrompt}
          placeholder="Agent prompt template (markdown)..."
          minHeight="240px"
          maxHeight="500px"
        />
      </div>
      <div className="flex gap-2 justify-end">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" size="sm">
          {isCreate ? 'Create' : 'Save'}
        </Button>
      </div>
    </form>
  )
}
