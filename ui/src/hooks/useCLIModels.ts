import { useQuery } from '@tanstack/react-query'
import { listCLIModels } from '@/api/cliModels'
import type { DropdownOption } from '@/components/ui/Dropdown'

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

export function useModelOptions(): DropdownOption[] {
  const { data: models = [] } = useCLIModels()
  return models
    .map((m) => ({ value: m.id, label: m.display_name }))
    .sort((a, b) => a.label.localeCompare(b.label))
}
