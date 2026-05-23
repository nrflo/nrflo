import { useQuery } from '@tanstack/react-query'
import { listAPIModels } from '@/api/apiModels'
import type { DropdownOptionGroup } from '@/components/ui/Dropdown'

const PROVIDER_LABELS: Record<string, string> = {
  anthropic: 'Anthropic',
  openai: 'OpenAI',
}

export const apiModelKeys = {
  all: ['api-models'] as const,
  list: () => [...apiModelKeys.all, 'list'] as const,
}

export function useAPIModels() {
  return useQuery({
    queryKey: apiModelKeys.list(),
    queryFn: listAPIModels,
  })
}

export function useAPIModelOptions(): DropdownOptionGroup[] {
  const { data: models = [] } = useAPIModels()
  if (models.length === 0) return []

  const grouped = new Map<string, { label: string; options: { value: string; label: string }[] }>()
  for (const m of models.filter(m => m.enabled)) {
    const groupLabel = PROVIDER_LABELS[m.provider] ?? m.provider.charAt(0).toUpperCase() + m.provider.slice(1)
    if (!grouped.has(m.provider)) {
      grouped.set(m.provider, { label: groupLabel, options: [] })
    }
    grouped.get(m.provider)!.options.push({ value: m.id, label: `${groupLabel}: ${m.display_name}` })
  }

  return [...grouped.values()]
    .sort((a, b) => a.label.localeCompare(b.label))
    .map((g) => ({ ...g, options: g.options.sort((a, b) => a.label.localeCompare(b.label)) }))
}
