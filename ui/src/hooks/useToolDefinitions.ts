import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listToolDefinitions,
  createToolDefinition,
  updateToolDefinition,
  deleteToolDefinition,
  type ListToolDefinitionsFilters,
} from '@/api/toolDefinitions'
import type { ToolDefinitionUpdateRequest } from '@/types/toolDefinition'

export const toolDefKeys = {
  all: ['tool-definitions'] as const,
  list: (filters?: ListToolDefinitionsFilters) => [...toolDefKeys.all, 'list', filters ?? {}] as const,
}

export function useToolDefinitions(filters?: ListToolDefinitionsFilters) {
  return useQuery({
    queryKey: toolDefKeys.list(filters),
    queryFn: () => listToolDefinitions(filters),
  })
}

export function useCreateToolDefinition() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createToolDefinition,
    onSuccess: () => qc.invalidateQueries({ queryKey: toolDefKeys.all }),
  })
}

export function useUpdateToolDefinition() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: ToolDefinitionUpdateRequest }) =>
      updateToolDefinition(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: toolDefKeys.all }),
  })
}

export function useDeleteToolDefinition() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deleteToolDefinition,
    onSuccess: () => qc.invalidateQueries({ queryKey: toolDefKeys.all }),
  })
}
