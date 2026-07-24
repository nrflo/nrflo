import { useState, useMemo } from 'react'
import { FileText } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { Toggle } from '@/components/ui/Toggle'
import { MarkdownEditor } from '@/components/ui/MarkdownEditor'
import { TemplatePickerDialog } from './TemplatePickerDialog'
import { AgentDefAPIModeFields } from './AgentDefAPIModeFields'
import { AgentDefNodeRoleFields } from './AgentDefNodeRoleFields'
import { AgentDefModelTierFields } from './AgentDefModelTierFields'
import { AgentDefSystemTemplateField } from './AgentDefSystemTemplateField'
import { AgentDefToolsField } from './AgentDefToolsField'
import { AgentDefNativeToolsField } from './AgentDefNativeToolsField'
import { AgentDefSandboxField } from './AgentDefSandboxField'
import { PythonScriptPickerField } from './PythonScriptPickerField'
import { AgentDefStepwiseSection } from './AgentDefStepwiseSection'
import { useModelOptions, useModels } from '@/hooks/useModels'
import { useAPIModeEnabled } from '@/hooks/useGlobalSettings'
import { validateStepDefinitions } from '@/lib/stepDefinitions'
import type { AgentDef, AgentDefCreateRequest, AgentDefUpdateRequest, PromptMode, StepDefinition } from '@/types/workflow'

type ExecutionMode = 'cli_interactive' | 'api' | 'script'
type NodeRole = 'static' | 'planner' | 'fanout_template'

export function AgentDefForm({
  initial,
  onSubmit,
  onCancel,
  isCreate,
  groups = [],
  submitError,
}: {
  initial?: Partial<AgentDef>
  onSubmit: (data: AgentDefCreateRequest | AgentDefUpdateRequest) => void
  onCancel: () => void
  isCreate: boolean
  groups?: string[]
  submitError?: string
}) {
  const [id, setId] = useState(initial?.id || '')
  const [layer, setLayer] = useState(initial?.layer ?? 0)
  const [model, setModel] = useState(initial?.model || 'sonnet-5')
  const [tier, setTier] = useState(initial?.tier ?? 1)
  const [override, setOverride] = useState(!!initial?.model)
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
  const [nativeTools, setNativeTools] = useState(initial?.native_tools || '')
  const [sandbox, setSandbox] = useState<string>(initial?.sandbox || '')
  const [apiMaxIterations, setApiMaxIterations] = useState<number | ''>(initial?.api_max_iterations ?? '')
  const [apiMaxTokens, setApiMaxTokens] = useState<number | ''>(initial?.api_max_tokens ?? '')
  const [validationCommands, setValidationCommands] = useState<string[]>(() => {
    try { return JSON.parse(initial?.validation_commands ?? '[]') } catch { return [] }
  })
  const [consultant, setConsultant] = useState(initial?.consultant ?? false)
  const [nodeRole, setNodeRole] = useState<NodeRole>((initial?.node_role as NodeRole) || 'static')
  const [description, setDescription] = useState(initial?.description || '')
  const [reasoningEffort, setReasoningEffort] = useState(initial?.reasoning_effort ?? '')
  const [systemTemplateId, setSystemTemplateId] = useState(initial?.system_template_id ?? '')
  const [promptMode, setPromptMode] = useState<PromptMode>((initial?.prompt_mode as PromptMode) || 'full')
  const [steps, setSteps] = useState<StepDefinition[]>(() => {
    try { return JSON.parse(initial?.steps ?? '[]') } catch { return [] }
  })
  const [showTemplatePicker, setShowTemplatePicker] = useState(false)
  const modelOptions = useModelOptions('cli')
  const apiModelOptions = useModelOptions('api')
  const activeModelOptions = executionMode === 'api' ? apiModelOptions : modelOptions
  const apiModeEnabled = useAPIModeEnabled()
  const { data: allModels = [] } = useModels()
  const providerOf = (modelID: string) => allModels.find((row) => row.id === modelID)?.provider
  const provider = providerOf(model)
  // Backend hard-rejects native_tools/sandbox that don't match the def's
  // provider/mode, so clear them whenever the model or mode moves away.
  const clearNativeFieldsFor = (nextModel: string, nextMode: ExecutionMode) => {
    const nextProvider = nextMode === 'cli_interactive' ? providerOf(nextModel) : undefined
    if (nextProvider !== 'anthropic') setNativeTools('')
    if (nextProvider !== 'openai') setSandbox('')
  }
  const handleModelChange = (nextModel: string) => {
    clearNativeFieldsFor(nextModel, executionMode)
    setModel(nextModel)
  }
  const handleExecutionModeChange = (v: string) => {
    const next = v as ExecutionMode
    if (next !== 'script') setPythonScriptId('')
    if (next === 'script') {
      setPromptMode('full')
      setSteps([])
    }
    let nextModel = model
    if (next !== 'script') {
      const options = next === 'api' ? apiModelOptions : modelOptions
      const values = options.flatMap((group) => group.options.map((option) => option.value))
      if (!values.includes(model)) {
        nextModel = values[0] ?? ''
        setModel(nextModel)
      }
      setLowConsumptionModel('')
      setReasoningEffort('')
    }
    clearNativeFieldsFor(nextModel, next)
    setExecutionMode(next)
  }
  const handleConsultantChange = (checked: boolean) => {
    setConsultant(checked)
    if (checked) setExecutionMode('api')
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (executionMode !== 'script' && isCreate && !prompt.trim()) return
    if (executionMode === 'script' && !pythonScriptId) return
    if (nodeRole === 'fanout_template' && !description.trim()) return
    if (executionMode !== 'script' && promptMode === 'stepwise' && validateStepDefinitions(steps).length > 0) return

    const threshold = restartThreshold !== '' ? restartThreshold : undefined
    const failRestarts = maxFailRestarts !== '' ? maxFailRestarts : undefined
    const tagValue = tag || undefined
    const trimmedCmds = validationCommands.map(s => s.trim()).filter(Boolean).slice(0, 20)
    const nodeRoleValue = nodeRole !== 'static' ? nodeRole : undefined
    const descriptionValue = description.trim() || undefined

    if (executionMode === 'script') {
      const base = { layer, timeout, restart_threshold: threshold, max_fail_restarts: failRestarts, tag: tagValue, execution_mode: 'script' as const, python_script_id: pythonScriptId, validation_commands: trimmedCmds, node_role: nodeRoleValue, description: descriptionValue }
      onSubmit(isCreate ? ({ id, ...base } as AgentDefCreateRequest) : (base as AgentDefUpdateRequest))
      return
    }

    const maxIter = apiMaxIterations !== '' ? apiMaxIterations : undefined
    const maxTokens = apiMaxTokens !== '' ? apiMaxTokens : undefined
    const lcModel = lowConsumptionModel || undefined
    const modelValue = override ? model : ''
    const tierValue = override ? null : tier
    const base = { layer, model: modelValue, tier: tierValue, timeout, prompt, restart_threshold: threshold, max_fail_restarts: failRestarts, tag: tagValue, low_consumption_model: lcModel, execution_mode: executionMode, tools, native_tools: nativeTools, sandbox: sandbox as AgentDefCreateRequest['sandbox'], api_max_iterations: maxIter, api_max_tokens: maxTokens, validation_commands: trimmedCmds, consultant: consultant || undefined, node_role: nodeRoleValue, description: descriptionValue, reasoning_effort: reasoningEffort || null, system_template_id: isCreate ? (systemTemplateId || undefined) : systemTemplateId, prompt_mode: promptMode, ...(promptMode === 'stepwise' ? { steps } : {}) }
    onSubmit(isCreate ? ({ id, ...base } as AgentDefCreateRequest) : (base as AgentDefUpdateRequest))
  }

  const executionModeOptions = useMemo(() => [
    { value: 'cli_interactive', label: 'CLI Interactive (PTY)' },
    ...(apiModeEnabled ? [{ value: 'api', label: 'API (in-process Anthropic runner)' }] : []),
    { value: 'script', label: 'Script (Python)' },
  ], [apiModeEnabled])

  return (
    <form onSubmit={handleSubmit} className="space-y-3 p-4 border border-border rounded-lg bg-muted/30">
      <div className="flex items-center gap-4">
        <Toggle checked={consultant} onChange={handleConsultantChange} label="Consultant" />
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Execution Mode</label>
        <Dropdown value={executionMode} onChange={handleExecutionModeChange} options={executionModeOptions} disabled={consultant} />
      </div>
      <AgentDefNodeRoleFields nodeRole={nodeRole} setNodeRole={setNodeRole} description={description} setDescription={setDescription} />
      {isCreate && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">ID</label>
          <input type="text" value={id} onChange={(e) => setId(e.target.value)} placeholder="e.g., setup-analyzer" required className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm" />
        </div>
      )}
      {!consultant && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">Layer</label>
          <input type="number" value={layer} onChange={(e) => setLayer(Number(e.target.value))} min={0} className="w-32 rounded-md border border-border bg-background px-3 py-1.5 text-sm" />
          <p className="text-xs text-muted-foreground mt-1">Execution order. Layer 0 runs first. Same-layer agents run concurrently.</p>
        </div>
      )}
      {executionMode !== 'script' && (
        <AgentDefModelTierFields
          tier={tier}
          onTierChange={setTier}
          override={override}
          onOverrideChange={setOverride}
          model={model}
          onModelChange={handleModelChange}
          executionMode={executionMode}
          reasoningEffort={reasoningEffort}
          onReasoningEffortChange={setReasoningEffort}
          modelOptions={activeModelOptions}
        />
      )}
      <div className="flex gap-3">
        {executionMode !== 'script' && (
          <AgentDefSystemTemplateField value={systemTemplateId} onChange={setSystemTemplateId} />
        )}
        <div className="w-32">
          <label className="block text-xs font-medium text-muted-foreground mb-1">Timeout (min)</label>
          <input type="number" value={timeout} onChange={(e) => setTimeout(Number(e.target.value))} min={1} className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm" />
        </div>
        {executionMode !== 'script' && !consultant && (
          <div className="w-32">
            <label className="block text-xs font-medium text-muted-foreground mb-1">Restart % (ctx)</label>
            <input type="number" value={restartThreshold} onChange={(e) => setRestartThreshold(e.target.value === '' ? '' : Number(e.target.value))} placeholder="25" min={1} max={99} className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm" />
          </div>
        )}
        {!consultant && (
          <div className="w-32">
            <label className="block text-xs font-medium text-muted-foreground mb-1">Fail restarts</label>
            <input type="number" value={maxFailRestarts} onChange={(e) => setMaxFailRestarts(e.target.value === '' ? '' : Number(e.target.value))} placeholder="0" min={0} max={10} className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm" />
          </div>
        )}
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
          <Dropdown value={lowConsumptionModel} onChange={setLowConsumptionModel} options={[{ label: '', options: [{ value: '', label: '(none)' }] }, ...activeModelOptions]} placeholder="(none)" />
          <p className="text-xs text-muted-foreground mt-1">Model to use when low consumption mode is enabled</p>
        </div>
      )}
      {executionMode !== 'script' && (
        <AgentDefToolsField value={tools} onChange={setTools} executionMode={executionMode} />
      )}
      {executionMode === 'cli_interactive' && provider === 'anthropic' && (
        <AgentDefNativeToolsField value={nativeTools} onChange={setNativeTools} />
      )}
      {executionMode === 'cli_interactive' && provider === 'openai' && (
        <AgentDefSandboxField value={sandbox} onChange={setSandbox} />
      )}
      {executionMode === 'api' && (
        <AgentDefAPIModeFields apiMaxIterations={apiMaxIterations} setApiMaxIterations={setApiMaxIterations} apiMaxTokens={apiMaxTokens} setApiMaxTokens={setApiMaxTokens} />
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
      {executionMode !== 'script' && (
        <AgentDefStepwiseSection promptMode={promptMode} onPromptModeChange={setPromptMode} steps={steps} onStepsChange={setSteps} />
      )}
      {submitError && (
        <p className="text-xs text-destructive">{submitError}</p>
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
