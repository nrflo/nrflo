import { useQuery } from '@tanstack/react-query'
import { getSummary, getEditRate, getThroughput } from '@/api/insights'
import type { InsightsRange, InsightsBucket } from '@/types/insights'

export const insightsKeys = {
  all: ['insights'] as const,
  summary: (range: InsightsRange) => ['insights', 'summary', range] as const,
  editRate: (range: InsightsRange) => ['insights', 'editRate', range] as const,
  throughput: (range: InsightsRange, bucket: InsightsBucket) =>
    ['insights', 'throughput', range, bucket] as const,
}

export function useInsightsSummary(range: InsightsRange) {
  return useQuery({
    queryKey: insightsKeys.summary(range),
    queryFn: () => getSummary(range),
  })
}

export function useInsightsEditRate(range: InsightsRange) {
  return useQuery({
    queryKey: insightsKeys.editRate(range),
    queryFn: () => getEditRate(range),
  })
}

export function useInsightsThroughput(range: InsightsRange, bucket: InsightsBucket) {
  return useQuery({
    queryKey: insightsKeys.throughput(range, bucket),
    queryFn: () => getThroughput(range, bucket),
  })
}
