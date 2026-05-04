import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listReviewItems,
  getReviewItem,
  updateReviewDraft,
  approveReview,
  rejectReview,
} from '@/api/review'

export const reviewKeys = {
  all: ['review'] as const,
  list: (status?: string) => ['review', 'list', status ?? 'all'] as const,
  detail: (id: string) => ['review', 'detail', id] as const,
}

export function useReviewItems(status?: string) {
  return useQuery({
    queryKey: reviewKeys.list(status),
    queryFn: () => listReviewItems({ status }),
  })
}

export function useReviewItem(id: string) {
  return useQuery({
    queryKey: reviewKeys.detail(id),
    queryFn: () => getReviewItem(id),
    enabled: !!id,
  })
}

export function useUpdateReviewDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, draft }: { id: string; draft: Record<string, unknown> }) =>
      updateReviewDraft(id, draft),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: reviewKeys.detail(id) })
    },
  })
}

export function useApproveReview() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => approveReview(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: reviewKeys.all }),
  })
}

export function useRejectReview() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) => rejectReview(id, reason),
    onSuccess: () => qc.invalidateQueries({ queryKey: reviewKeys.all }),
  })
}
