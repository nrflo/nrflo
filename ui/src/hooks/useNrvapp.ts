import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listReviewItems,
  getReviewItem,
  updateReviewDraft,
  approveReview,
  rejectReview,
  listConfigFiles,
  getConfigFile,
  putConfigFile,
  getConfigHistory,
  rollbackConfig,
  getSummary,
  getEditRate,
  getThroughput,
} from '@/api/nrvapp'
import type { NrvappRange, NrvappBucket } from '@/types/nrvapp'

export const nrvappKeys = {
  all: ['nrvapp'] as const,
  review: {
    all: ['nrvapp', 'review'] as const,
    list: (status?: string) => ['nrvapp', 'review', 'list', status ?? 'all'] as const,
    detail: (id: string) => ['nrvapp', 'review', 'detail', id] as const,
  },
  config: {
    all: ['nrvapp', 'config'] as const,
    files: ['nrvapp', 'config', 'files'] as const,
    content: (path: string) => ['nrvapp', 'config', 'content', path] as const,
    history: (path: string) => ['nrvapp', 'config', 'history', path] as const,
  },
  insights: {
    all: ['nrvapp', 'insights'] as const,
    summary: (range: NrvappRange) => ['nrvapp', 'insights', 'summary', range] as const,
    editRate: (range: NrvappRange) => ['nrvapp', 'insights', 'editRate', range] as const,
    throughput: (range: NrvappRange, bucket: NrvappBucket) =>
      ['nrvapp', 'insights', 'throughput', range, bucket] as const,
  },
}

export function useReviewItems(status?: string) {
  return useQuery({
    queryKey: nrvappKeys.review.list(status),
    queryFn: () => listReviewItems({ status }),
  })
}

export function useReviewItem(id: string) {
  return useQuery({
    queryKey: nrvappKeys.review.detail(id),
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
      qc.invalidateQueries({ queryKey: nrvappKeys.review.detail(id) })
    },
  })
}

export function useApproveReview() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => approveReview(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: nrvappKeys.review.all }),
  })
}

export function useRejectReview() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) => rejectReview(id, reason),
    onSuccess: () => qc.invalidateQueries({ queryKey: nrvappKeys.review.all }),
  })
}

export function useConfigFiles() {
  return useQuery({
    queryKey: nrvappKeys.config.files,
    queryFn: listConfigFiles,
  })
}

export function useConfigFile(path: string) {
  return useQuery({
    queryKey: nrvappKeys.config.content(path),
    queryFn: () => getConfigFile(path),
    enabled: !!path,
  })
}

export function usePutConfigFile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ path, content }: { path: string; content: string }) =>
      putConfigFile(path, content),
    onSuccess: (_data, { path }) => {
      qc.invalidateQueries({ queryKey: nrvappKeys.config.content(path) })
      qc.invalidateQueries({ queryKey: nrvappKeys.config.history(path) })
      qc.invalidateQueries({ queryKey: nrvappKeys.config.files })
    },
  })
}

export function useConfigHistory(path: string) {
  return useQuery({
    queryKey: nrvappKeys.config.history(path),
    queryFn: () => getConfigHistory(path),
    enabled: !!path,
  })
}

export function useRollbackConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ path, version }: { path: string; version: number }) =>
      rollbackConfig(path, version),
    onSuccess: (_data, { path }) => {
      qc.invalidateQueries({ queryKey: nrvappKeys.config.content(path) })
      qc.invalidateQueries({ queryKey: nrvappKeys.config.history(path) })
      qc.invalidateQueries({ queryKey: nrvappKeys.config.files })
    },
  })
}

export function useNrvappSummary(range: NrvappRange) {
  return useQuery({
    queryKey: nrvappKeys.insights.summary(range),
    queryFn: () => getSummary(range),
  })
}

export function useNrvappEditRate(range: NrvappRange) {
  return useQuery({
    queryKey: nrvappKeys.insights.editRate(range),
    queryFn: () => getEditRate(range),
  })
}

export function useNrvappThroughput(range: NrvappRange, bucket: NrvappBucket) {
  return useQuery({
    queryKey: nrvappKeys.insights.throughput(range, bucket),
    queryFn: () => getThroughput(range, bucket),
  })
}
