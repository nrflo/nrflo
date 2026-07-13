import { Dropdown } from '@/components/ui/Dropdown'
import { useCLIModels } from '@/hooks/useCLIModels'
import { useAPIModels } from '@/hooks/useAPIModels'
import { buildCLIEffortOptions, buildAPIEffortOptions } from '@/components/settings/effortOptions'

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
  const { data: cliModels = [] } = useCLIModels()
  const { data: apiModels = [] } = useAPIModels()

  if (executionMode === 'script') return null

  const isApi = executionMode === 'api'
  const apiRow = apiModels.find((m) => m.id === model)
  const cliRow = cliModels.find((m) => m.id === model)
  const options = isApi
    ? buildAPIEffortOptions(apiRow?.provider ?? 'anthropic', apiRow?.mapped_model ?? '')
    : buildCLIEffortOptions(cliRow?.cli_type ?? 'claude', cliRow?.mapped_model ?? '')

  const inherited = (isApi ? apiRow?.reasoning_effort : cliRow?.reasoning_effort) || 'none'
  const inheritOptions = options.map((opt) =>
    opt.value === '' ? { ...opt, label: `Inherit from model (${inherited})` } : opt
  )

  return (
    <div className="flex-1">
      <label className="block text-xs font-medium text-muted-foreground mb-1">Reasoning Effort</label>
      <Dropdown value={value} onChange={onChange} options={inheritOptions} />
    </div>
  )
}
