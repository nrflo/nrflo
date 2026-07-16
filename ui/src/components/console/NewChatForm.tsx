import { useState } from 'react'
import { Dropdown } from '@/components/ui/Dropdown'
import { ProjectSelect } from '@/components/ui/ProjectSelect'
import { Button } from '@/components/ui/Button'
import { useConsoleCatalog, useCreateConsoleChat } from '@/hooks/useConsoleChats'
import { useProjectStore } from '@/stores/projectStore'

interface NewChatFormProps {
  onCreated: (sid: string) => void
}

// Server-driven picker over GET /console/catalog — the same discovery
// surface the native TUI uses. Engine availability (codex missing on the
// server, API mode off) arrives as enabled/disabled_reason instead of being
// re-derived client-side; each engine carries its own registry's models
// (cli_models vs api_models — colliding id namespaces). CLI engines accept
// an empty model (engine default); the api engine requires one
// (requires_model). The read-only workdir line shows project.root_path —
// exactly what buildChatEngineSpec uses as the engine WorkDir.
export function NewChatForm({ onCreated }: NewChatFormProps) {
  const projects = useProjectStore((s) => s.projects)
  const currentProject = useProjectStore((s) => s.currentProject)
  const setCurrentProject = useProjectStore((s) => s.setCurrentProject)
  const { data: catalog } = useConsoleCatalog()
  const createMutation = useCreateConsoleChat()

  const [engine, setEngine] = useState('claude')
  const [model, setModel] = useState('')
  const [effort, setEffort] = useState('')

  const engines = catalog?.engines ?? []
  const selectedEngine = engines.find((e) => e.id === engine)
  const selectedModel = selectedEngine?.models?.find((m) => m.id === model)
  const supportedEfforts = selectedModel?.supported_efforts ?? []

  // If the chosen engine turns disabled under us (e.g. API mode flipped
  // off), snap to the first enabled one.
  const firstEnabled = engines.find((e) => e.enabled)
  if (selectedEngine && !selectedEngine.enabled && firstEnabled) {
    setEngine(firstEnabled.id)
    setModel('')
  }

  const engineOptions = engines.map((e) => ({
    value: e.id,
    label: e.display_name,
    disabled: !e.enabled,
    tooltip: e.enabled ? undefined : e.disabled_reason,
  }))

  const modelOptions = (selectedEngine?.models ?? []).map((m) => ({
    value: m.id,
    label: m.display_name,
  }))

  const project = projects.find((p) => p.id === currentProject)

  const handleEngineChange = (value: string) => {
    setEngine(value)
    setModel('')
    setEffort('')
  }

  const handleModelChange = (value: string) => {
    setModel(value)
    setEffort('')
  }

  const effortOptions = [
    { value: '', label: selectedModel?.reasoning_effort ? `Default (${selectedModel.reasoning_effort})` : 'Default' },
    ...supportedEfforts.map((e) => ({ value: e, label: e.charAt(0).toUpperCase() + e.slice(1) })),
  ]

  const canCreate =
    !!selectedEngine?.enabled && (!!model || !selectedEngine.requires_model)

  const handleCreate = async () => {
    if (!canCreate) return
    const resp = await createMutation.mutateAsync({
      engine,
      model,
      ...(effort ? { reasoning_effort: effort } : {}),
    })
    onCreated(resp.session_id)
  }

  return (
    <div className="flex flex-col gap-3 border-b border-border p-3">
      <div>
        <label className="mb-1 block text-xs font-medium text-muted-foreground">Engine</label>
        <Dropdown value={engine} onChange={handleEngineChange} options={engineOptions} />
      </div>
      <div>
        <label className="mb-1 block text-xs font-medium text-muted-foreground">Model</label>
        <Dropdown
          value={model}
          onChange={handleModelChange}
          options={modelOptions}
          placeholder={selectedEngine?.requires_model ? 'Select a model…' : 'Engine default'}
          disabled={modelOptions.length === 0}
        />
      </div>
      {supportedEfforts.length > 0 && (
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">Reasoning effort</label>
          <Dropdown value={effort} onChange={setEffort} options={effortOptions} />
        </div>
      )}
      {engine === 'api' && (
        <div className="text-xs text-muted-foreground">
          No file/edit/bash tools — nrflo control + web research only; use a CLI engine for local coding.
        </div>
      )}
      <div>
        <label className="mb-1 block text-xs font-medium text-muted-foreground">Project</label>
        <ProjectSelect value={currentProject} onChange={setCurrentProject} projects={projects} />
      </div>
      {project?.root_path && <div className="text-xs text-muted-foreground">Workdir: {project.root_path}</div>}
      <Button onClick={handleCreate} disabled={!canCreate || createMutation.isPending}>
        {createMutation.isPending ? 'Starting…' : 'New chat'}
      </Button>
    </div>
  )
}
