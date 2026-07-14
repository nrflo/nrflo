import { useState } from 'react'
import { Dropdown } from '@/components/ui/Dropdown'
import { ProjectSelect } from '@/components/ui/ProjectSelect'
import { Button } from '@/components/ui/Button'
import { useCLIModels } from '@/hooks/useCLIModels'
import { useCreateConsoleChat } from '@/hooks/useConsoleChats'
import { useProjectStore } from '@/stores/projectStore'

interface NewChatFormProps {
  onCreated: (sid: string) => void
}

const ENGINE_OPTIONS = [
  { value: 'claude', label: 'Claude' },
  { value: 'codex', label: 'Codex' },
]

// Engine picker, model picker filtered to the chosen engine's cli_type +
// enabled rows (the BE rejects a model for the other engine or a disabled one
// at create time, so filtering client-side avoids a guaranteed 500), project
// picker, and a read-only workdir line — project.root_path is exactly what
// buildChatEngineSpec uses as the engine WorkDir.
export function NewChatForm({ onCreated }: NewChatFormProps) {
  const projects = useProjectStore((s) => s.projects)
  const currentProject = useProjectStore((s) => s.currentProject)
  const setCurrentProject = useProjectStore((s) => s.setCurrentProject)
  const { data: models = [] } = useCLIModels()
  const createMutation = useCreateConsoleChat()

  const [engine, setEngine] = useState('claude')
  const [model, setModel] = useState('')

  const modelOptions = models
    .filter((m) => m.enabled && m.cli_type === engine)
    .map((m) => ({ value: m.id, label: m.display_name }))

  const project = projects.find((p) => p.id === currentProject)

  const handleEngineChange = (value: string) => {
    setEngine(value)
    setModel('')
  }

  const handleCreate = async () => {
    if (!model) return
    const resp = await createMutation.mutateAsync({ engine, model })
    onCreated(resp.session_id)
  }

  return (
    <div className="flex flex-col gap-3 border-b border-border p-3">
      <div>
        <label className="mb-1 block text-xs font-medium text-muted-foreground">Engine</label>
        <Dropdown value={engine} onChange={handleEngineChange} options={ENGINE_OPTIONS} />
      </div>
      <div>
        <label className="mb-1 block text-xs font-medium text-muted-foreground">Model</label>
        <Dropdown
          value={model}
          onChange={setModel}
          options={modelOptions}
          placeholder="Select a model…"
          disabled={modelOptions.length === 0}
        />
      </div>
      <div>
        <label className="mb-1 block text-xs font-medium text-muted-foreground">Project</label>
        <ProjectSelect value={currentProject} onChange={setCurrentProject} projects={projects} />
      </div>
      {project?.root_path && <div className="text-xs text-muted-foreground">Workdir: {project.root_path}</div>}
      <Button onClick={handleCreate} disabled={!model || createMutation.isPending}>
        {createMutation.isPending ? 'Starting…' : 'New chat'}
      </Button>
    </div>
  )
}
