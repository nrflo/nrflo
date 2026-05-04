import { apiGet } from './client'
import type { InsightsSummary, EditRateRow, ThroughputPoint, InsightsRange, InsightsBucket } from '@/types/insights'

export async function getSummary(range: InsightsRange): Promise<InsightsSummary> {
  return apiGet<InsightsSummary>(`/api/v1/insights/summary?range=${range}`)
}

export async function getEditRate(range: InsightsRange): Promise<EditRateRow[]> {
  return apiGet<EditRateRow[]>(`/api/v1/insights/edit-rate?range=${range}`)
}

export async function getThroughput(
  range: InsightsRange,
  bucket: InsightsBucket
): Promise<ThroughputPoint[]> {
  return apiGet<ThroughputPoint[]>(
    `/api/v1/insights/throughput?range=${range}&bucket=${bucket}`
  )
}
