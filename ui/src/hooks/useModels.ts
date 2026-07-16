import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createModel,
  deleteModel,
  listModels,
  updateModel,
  type CreateModelRequest,
  type ModelMode,
  type UpdateModelRequest,
} from '@/api/models'
import type { DropdownOptionGroup } from '@/components/ui/Dropdown'

const PROVIDER_LABELS = { anthropic: 'Anthropic', openai: 'OpenAI' } as const

export function cliTypeForProvider(provider: 'anthropic' | 'openai') {
  return provider === 'anthropic' ? 'claude' : 'codex'
}

export const modelKeys = {
  all: ['models'] as const,
  list: () => [...modelKeys.all, 'list'] as const,
}

export function useModels() {
  return useQuery({ queryKey: modelKeys.list(), queryFn: listModels })
}

export function useCreateModel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateModelRequest) => createModel(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: modelKeys.list() }),
  })
}

export function useUpdateModel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateModelRequest }) => updateModel(id, data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: modelKeys.list() }),
  })
}

export function useDeleteModel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: deleteModel,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: modelKeys.list() }),
  })
}

export function useModelOptions(mode: ModelMode): DropdownOptionGroup[] {
  const { data: models = [] } = useModels()
  const modeField = mode === 'api' ? 'api_model' : 'cli_model'
  const grouped = new Map<string, DropdownOptionGroup>()

  for (const model of models.filter((row) => row.enabled && row[modeField])) {
    const label = PROVIDER_LABELS[model.provider]
    const group = grouped.get(model.provider) ?? { label, options: [] }
    group.options.push({ value: model.id, label: `${label}: ${model.display_name}` })
    grouped.set(model.provider, group)
  }

  return [...grouped.values()]
    .sort((a, b) => a.label.localeCompare(b.label))
    .map((group) => ({
      ...group,
      options: [...group.options].sort((a, b) => a.label.localeCompare(b.label)),
    }))
}
