import { useState } from 'react'
import { FileText } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { MarkdownEditor } from '@/components/ui/MarkdownEditor'
import { TemplatePickerDialog } from './TemplatePickerDialog'
import { useModelOptions } from '@/hooks/useCLIModels'
import type { AgentDef, AgentDefCreateRequest, AgentDefUpdateRequest } from '@/types/workflow'

export function AgentDefForm({
  initial,
  onSubmit,
  onCancel,
  isCreate,
  groups = [],
}: {
  initial?: Partial<AgentDef>
  onSubmit: (data: AgentDefCreateRequest | AgentDefUpdateRequest) => void
  onCancel: () => void
  isCreate: boolean
  groups?: string[]
}) {
  const [id, setId] = useState(initial?.id || '')
  const [model, setModel] = useState(initial?.model || 'sonnet')
  const [timeout, setTimeout] = useState(initial?.timeout || 20)
  const [restartThreshold, setRestartThreshold] = useState<number | ''>(initial?.restart_threshold ?? '')
  const [maxFailRestarts, setMaxFailRestarts] = useState<number | ''>(initial?.max_fail_restarts ?? '')
  const [tag, setTag] = useState(initial?.tag || '')
  const [lowConsumptionModel, setLowConsumptionModel] = useState(initial?.low_consumption_model || '')
  const [prompt, setPrompt] = useState(initial?.prompt || '')
  const [showTemplatePicker, setShowTemplatePicker] = useState(false)
  const modelOptions = useModelOptions()

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (isCreate && !prompt.trim()) return
    const threshold = restartThreshold !== '' ? restartThreshold : undefined
    const failRestarts = maxFailRestarts !== '' ? maxFailRestarts : undefined
    const tagValue = tag || undefined
    const lcModel = lowConsumptionModel || undefined
    if (isCreate) {
      onSubmit({ id, model, timeout, prompt, restart_threshold: threshold, max_fail_restarts: failRestarts, tag: tagValue, low_consumption_model: lcModel } as AgentDefCreateRequest)
    } else {
      onSubmit({ model, timeout, prompt, restart_threshold: threshold, max_fail_restarts: failRestarts, tag: tagValue, low_consumption_model: lcModel } as AgentDefUpdateRequest)
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
            options={modelOptions}
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
        <div className="w-32">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Fail restarts</label>
          <input
            type="number"
            value={maxFailRestarts}
            onChange={(e) => setMaxFailRestarts(e.target.value === '' ? '' : Number(e.target.value))}
            placeholder="0"
            min={0}
            max={10}
            className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
          />
        </div>
      </div>
      {groups.length > 0 && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">Tag</label>
          <Dropdown
            value={tag}
            onChange={setTag}
            options={[
              { value: '', label: '(none)' },
              ...groups.map((g) => ({ value: g, label: g })),
            ]}
            placeholder="(none)"
          />
          <p className="text-xs text-muted-foreground mt-1">
            Assign a group tag for skip logic (optional)
          </p>
        </div>
      )}
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Low consumption model</label>
        <Dropdown
          value={lowConsumptionModel}
          onChange={setLowConsumptionModel}
          options={[{ label: '', options: [{ value: '', label: '(none)' }] }, ...modelOptions]}
          placeholder="(none)"
        />
        <p className="text-xs text-muted-foreground mt-1">
          Model to use when low consumption mode is enabled
        </p>
      </div>
      <div>
        <div className="flex items-center justify-between mb-1">
          <label className="text-xs font-medium text-muted-foreground">Prompt Template</label>
          <Button type="button" variant="ghost" size="sm" onClick={() => setShowTemplatePicker(true)}>
            <FileText className="h-3.5 w-3.5 mr-1" />
            Apply Template
          </Button>
        </div>
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
      {showTemplatePicker && (
        <TemplatePickerDialog
          open={showTemplatePicker}
          onClose={() => setShowTemplatePicker(false)}
          onApply={setPrompt}
          hasExistingPrompt={prompt.trim().length > 0}
        />
      )}
    </form>
  )
}
