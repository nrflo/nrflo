import { useState, useMemo } from 'react'
import { FileText } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { MarkdownEditor } from '@/components/ui/MarkdownEditor'
import { TemplatePickerDialog } from './TemplatePickerDialog'
import { AgentDefAPIModeFields } from './AgentDefAPIModeFields'
import { PythonScriptPickerField } from './PythonScriptPickerField'
import { useModelOptions } from '@/hooks/useCLIModels'
import { useAPIModeEnabled } from '@/hooks/useGlobalSettings'
import type { AgentDef, AgentDefCreateRequest, AgentDefUpdateRequest } from '@/types/workflow'

type ExecutionMode = 'cli_interactive' | 'api' | 'script'

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
  const [layer, setLayer] = useState(initial?.layer ?? 0)
  const [model, setModel] = useState(initial?.model || 'sonnet')
  const [timeout, setTimeout] = useState(initial?.timeout || 20)
  const [restartThreshold, setRestartThreshold] = useState<number | ''>(initial?.restart_threshold ?? '')
  const [maxFailRestarts, setMaxFailRestarts] = useState<number | ''>(initial?.max_fail_restarts ?? '')
  const [tag, setTag] = useState(initial?.tag || '')
  const [lowConsumptionModel, setLowConsumptionModel] = useState(initial?.low_consumption_model || '')
  const [prompt, setPrompt] = useState(initial?.prompt || '')
  const [executionMode, setExecutionMode] = useState<ExecutionMode>(
    (initial?.execution_mode as ExecutionMode) || 'cli_interactive'
  )
  const [pythonScriptId, setPythonScriptId] = useState(initial?.python_script_id || '')
  const [tools, setTools] = useState(initial?.tools || '')
  const [apiMaxIterations, setApiMaxIterations] = useState<number | ''>(initial?.api_max_iterations ?? '')
  const [validationCommands, setValidationCommands] = useState<string[]>(() => {
    try { return JSON.parse(initial?.validation_commands ?? '[]') } catch { return [] }
  })
  const [showTemplatePicker, setShowTemplatePicker] = useState(false)
  const modelOptions = useModelOptions()
  const apiModeEnabled = useAPIModeEnabled()
  const handleExecutionModeChange = (v: string) => {
    const next = v as ExecutionMode
    if (next !== 'script') setPythonScriptId('')
    setExecutionMode(next)
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (executionMode !== 'script' && isCreate && !prompt.trim()) return
    if (executionMode === 'script' && !pythonScriptId) return

    const threshold = restartThreshold !== '' ? restartThreshold : undefined
    const failRestarts = maxFailRestarts !== '' ? maxFailRestarts : undefined
    const tagValue = tag || undefined
    const trimmedCmds = validationCommands.map(s => s.trim()).filter(Boolean).slice(0, 20)

    if (executionMode === 'script') {
      const base = { layer, timeout, restart_threshold: threshold, max_fail_restarts: failRestarts, tag: tagValue, execution_mode: 'script' as const, python_script_id: pythonScriptId, validation_commands: trimmedCmds }
      onSubmit(isCreate ? ({ id, ...base } as AgentDefCreateRequest) : (base as AgentDefUpdateRequest))
      return
    }

    const maxIter = apiMaxIterations !== '' ? apiMaxIterations : undefined
    const lcModel = lowConsumptionModel || undefined
    const base = { layer, model, timeout, prompt, restart_threshold: threshold, max_fail_restarts: failRestarts, tag: tagValue, low_consumption_model: lcModel, execution_mode: executionMode, tools, api_max_iterations: maxIter, validation_commands: trimmedCmds }
    onSubmit(isCreate ? ({ id, ...base } as AgentDefCreateRequest) : (base as AgentDefUpdateRequest))
  }

  const executionModeOptions = useMemo(() => [
    { value: 'cli_interactive', label: 'CLI Interactive (PTY)' },
    ...(apiModeEnabled ? [{ value: 'api', label: 'API (in-process Anthropic runner)' }] : []),
    { value: 'script', label: 'Script (Python)' },
  ], [apiModeEnabled])

  return (
    <form onSubmit={handleSubmit} className="space-y-3 p-4 border border-border rounded-lg bg-muted/30">
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Execution Mode</label>
        <Dropdown value={executionMode} onChange={handleExecutionModeChange} options={executionModeOptions} />
      </div>
      {isCreate && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">ID</label>
          <input type="text" value={id} onChange={(e) => setId(e.target.value)} placeholder="e.g., setup-analyzer" required className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm" />
        </div>
      )}
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Layer</label>
        <input type="number" value={layer} onChange={(e) => setLayer(Number(e.target.value))} min={0} className="w-32 rounded-md border border-border bg-background px-3 py-1.5 text-sm" />
        <p className="text-xs text-muted-foreground mt-1">Execution order. Layer 0 runs first. Same-layer agents run concurrently.</p>
      </div>
      <div className="flex gap-3">
        {executionMode !== 'script' && (
          <div className="flex-1">
            <label className="block text-xs font-medium text-muted-foreground mb-1">Model</label>
            <Dropdown value={model} onChange={setModel} options={modelOptions} />
          </div>
        )}
        <div className="w-32">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Timeout (min)</label>
          <input type="number" value={timeout} onChange={(e) => setTimeout(Number(e.target.value))} min={1} className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm" />
        </div>
        {executionMode !== 'script' && (
          <div className="w-32">
            <label className="block text-xs font-medium text-muted-foreground mb-1">Restart % (ctx)</label>
            <input type="number" value={restartThreshold} onChange={(e) => setRestartThreshold(e.target.value === '' ? '' : Number(e.target.value))} placeholder="25" min={1} max={99} className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm" />
          </div>
        )}
        <div className="w-32">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Fail restarts</label>
          <input type="number" value={maxFailRestarts} onChange={(e) => setMaxFailRestarts(e.target.value === '' ? '' : Number(e.target.value))} placeholder="0" min={0} max={10} className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm" />
        </div>
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Validation commands</label>
        <p className="text-xs text-muted-foreground mb-2">Commands run by the orchestrator after the agent reports pass. Any non-zero exit fails the session.</p>
        <div className="space-y-1.5">
          {validationCommands.map((cmd, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <input
                type="text"
                value={cmd}
                onChange={(e) => setValidationCommands(prev => prev.map((c, i) => i === idx ? e.target.value : c))}
                placeholder="e.g., make test"
                className="flex-1 rounded-md border border-border bg-background px-3 py-1.5 text-sm"
              />
              <Button type="button" variant="ghost" size="sm" onClick={() => setValidationCommands(prev => prev.filter((_, i) => i !== idx))}>Remove</Button>
            </div>
          ))}
        </div>
        <Button type="button" variant="outline" size="sm" className="mt-2" disabled={validationCommands.length >= 20} onClick={() => setValidationCommands(prev => [...prev, ''])}>Add command</Button>
      </div>
      {groups.length > 0 && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">Tag</label>
          <Dropdown value={tag} onChange={setTag} options={[{ value: '', label: '(none)' }, ...groups.map((g) => ({ value: g, label: g }))]} placeholder="(none)" />
          <p className="text-xs text-muted-foreground mt-1">Assign a group tag for skip logic (optional)</p>
        </div>
      )}
      {executionMode !== 'script' && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">Low consumption model</label>
          <Dropdown value={lowConsumptionModel} onChange={setLowConsumptionModel} options={[{ label: '', options: [{ value: '', label: '(none)' }] }, ...modelOptions]} placeholder="(none)" />
          <p className="text-xs text-muted-foreground mt-1">Model to use when low consumption mode is enabled</p>
        </div>
      )}
      {executionMode === 'api' && (
        <AgentDefAPIModeFields tools={tools} setTools={setTools} apiMaxIterations={apiMaxIterations} setApiMaxIterations={setApiMaxIterations} />
      )}
      {executionMode === 'script' && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">
            Python Script <span className="text-destructive">*</span>
          </label>
          <PythonScriptPickerField value={pythonScriptId} onChange={setPythonScriptId} />
        </div>
      )}
      {executionMode !== 'script' && (
        <div>
          <div className="flex items-center justify-between mb-1">
            <label className="text-xs font-medium text-muted-foreground">Prompt Template</label>
            <Button type="button" variant="ghost" size="sm" onClick={() => setShowTemplatePicker(true)}>
              <FileText className="h-3.5 w-3.5 mr-1" />
              Apply Template
            </Button>
          </div>
          <MarkdownEditor value={prompt} onChange={setPrompt} placeholder="Agent prompt template (markdown)..." minHeight="240px" maxHeight="500px" />
        </div>
      )}
      <div className="flex gap-2 justify-end">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>Cancel</Button>
        <Button type="submit" size="sm">{isCreate ? 'Create' : 'Save'}</Button>
      </div>
      {showTemplatePicker && (
        <TemplatePickerDialog open={showTemplatePicker} onClose={() => setShowTemplatePicker(false)} onApply={setPrompt} hasExistingPrompt={prompt.trim().length > 0} />
      )}
    </form>
  )
}
