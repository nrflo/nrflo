import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { applyTiering, getTieringReport } from '@/api/tiering'
import type { TieringApplyRequest } from '@/types/tiering'

export const tieringKeys = {
  all: ['tiering'] as const,
  report: () => [...tieringKeys.all, 'report'] as const,
}

export function useTieringReport() {
  return useQuery({ queryKey: tieringKeys.report(), queryFn: getTieringReport })
}

export function useApplyTiering() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (payload: TieringApplyRequest) => applyTiering(payload),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: tieringKeys.report() }),
  })
}
