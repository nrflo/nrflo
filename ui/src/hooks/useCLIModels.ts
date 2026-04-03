import { useQuery } from '@tanstack/react-query'
import { listCLIModels } from '@/api/cliModels'
import type { DropdownOptionGroup } from '@/components/ui/Dropdown'

const CLI_TYPE_LABELS: Record<string, string> = {
  claude: 'Claude',
  opencode: 'OpenCode',
  codex: 'Codex',
}

export const cliModelKeys = {
  all: ['cli-models'] as const,
  list: () => [...cliModelKeys.all, 'list'] as const,
}

export function useCLIModels() {
  return useQuery({
    queryKey: cliModelKeys.list(),
    queryFn: listCLIModels,
  })
}

export function useModelOptions(): DropdownOptionGroup[] {
  const { data: models = [] } = useCLIModels()
  if (models.length === 0) return []

  const grouped = new Map<string, { label: string; options: { value: string; label: string }[] }>()
  for (const m of models.filter(m => m.enabled)) {
    const groupLabel = CLI_TYPE_LABELS[m.cli_type] ?? m.cli_type.charAt(0).toUpperCase() + m.cli_type.slice(1)
    if (!grouped.has(m.cli_type)) {
      grouped.set(m.cli_type, { label: groupLabel, options: [] })
    }
    grouped.get(m.cli_type)!.options.push({ value: m.id, label: `${groupLabel}: ${m.display_name}` })
  }

  return [...grouped.values()]
    .sort((a, b) => a.label.localeCompare(b.label))
    .map((g) => ({ ...g, options: g.options.sort((a, b) => a.label.localeCompare(b.label)) }))
}
