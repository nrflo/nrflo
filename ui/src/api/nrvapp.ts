import { apiGet, apiPatch, apiPost, apiFetch } from './client'
import type {
  NrvappReviewItem,
  NrvappConfigFileMeta,
  NrvappConfigFile,
  NrvappConfigVersion,
  NrvappSummary,
  NrvappEditRateRow,
  NrvappThroughputPoint,
  NrvappRange,
  NrvappBucket,
} from '@/types/nrvapp'

function encodePathSegments(path: string): string {
  return path.split('/').map(encodeURIComponent).join('/')
}

export async function listReviewItems(params?: {
  status?: string
  limit?: number
  offset?: number
}): Promise<NrvappReviewItem[]> {
  const p = new URLSearchParams()
  if (params?.status) p.set('status', params.status)
  if (params?.limit !== undefined) p.set('limit', String(params.limit))
  if (params?.offset !== undefined) p.set('offset', String(params.offset))
  const qs = p.toString()
  return apiGet<NrvappReviewItem[]>(`/api/v1/nrvapp/review${qs ? `?${qs}` : ''}`)
}

export async function getReviewItem(id: string): Promise<NrvappReviewItem> {
  return apiGet<NrvappReviewItem>(`/api/v1/nrvapp/review/${encodeURIComponent(id)}`)
}

export async function updateReviewDraft(
  id: string,
  draft: Record<string, unknown>
): Promise<NrvappReviewItem> {
  return apiPatch<NrvappReviewItem>(`/api/v1/nrvapp/review/${encodeURIComponent(id)}`, { draft })
}

export async function approveReview(id: string): Promise<NrvappReviewItem> {
  return apiPost<NrvappReviewItem>(`/api/v1/nrvapp/review/${encodeURIComponent(id)}/approve`)
}

export async function rejectReview(id: string, reason: string): Promise<NrvappReviewItem> {
  return apiPost<NrvappReviewItem>(`/api/v1/nrvapp/review/${encodeURIComponent(id)}/reject`, {
    reason,
  })
}

export async function listConfigFiles(): Promise<NrvappConfigFileMeta[]> {
  return apiGet<NrvappConfigFileMeta[]>('/api/v1/nrvapp/config/files')
}

export async function getConfigFile(path: string): Promise<NrvappConfigFile> {
  return apiGet<NrvappConfigFile>(`/api/v1/nrvapp/config/content/${encodePathSegments(path)}`)
}

export async function putConfigFile(path: string, content: string): Promise<NrvappConfigFile> {
  return apiFetch<NrvappConfigFile>(
    `/api/v1/nrvapp/config/content/${encodePathSegments(path)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain' },
      body: content,
    }
  )
}

export async function getConfigHistory(path: string): Promise<NrvappConfigVersion[]> {
  return apiGet<NrvappConfigVersion[]>(
    `/api/v1/nrvapp/config/history/${encodePathSegments(path)}`
  )
}

export async function rollbackConfig(path: string, version: number): Promise<NrvappConfigFile> {
  return apiPost<NrvappConfigFile>(
    `/api/v1/nrvapp/config/rollback/${encodePathSegments(path)}`,
    { version }
  )
}

export async function getSummary(range: NrvappRange): Promise<NrvappSummary> {
  return apiGet<NrvappSummary>(`/api/v1/nrvapp/insights/summary?range=${range}`)
}

export async function getEditRate(range: NrvappRange): Promise<NrvappEditRateRow[]> {
  return apiGet<NrvappEditRateRow[]>(`/api/v1/nrvapp/insights/edit-rate?range=${range}`)
}

export async function getThroughput(
  range: NrvappRange,
  bucket: NrvappBucket
): Promise<NrvappThroughputPoint[]> {
  return apiGet<NrvappThroughputPoint[]>(
    `/api/v1/nrvapp/insights/throughput?range=${range}&bucket=${bucket}`
  )
}
