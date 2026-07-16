import { Dropdown } from '@/components/ui/Dropdown'
import { useModels } from '@/hooks/useModels'

type ExecutionMode = 'cli_interactive' | 'api' | 'script'

export function AgentDefEffortField({
  executionMode,
  model,
  value,
  onChange,
}: {
  executionMode: ExecutionMode
  model: string
  value: string
  onChange: (v: string) => void
}) {
  const { data: models = [] } = useModels()

  if (executionMode === 'script') return null

  const row = models.find((item) => item.id === model)
  const efforts = executionMode === 'api' ? row?.api_efforts ?? [] : row?.cli_efforts ?? []
  const inherited = row?.default_effort || 'provider default'
  const options = [
    { value: '', label: `Inherit from model (${inherited})` },
    ...efforts.map((effort) => ({ value: effort, label: effort })),
  ]

  return (
    <div className="flex-1">
      <label className="block text-xs font-medium text-muted-foreground mb-1">Reasoning Effort</label>
      <Dropdown value={value} onChange={onChange} options={options} />
    </div>
  )
}
