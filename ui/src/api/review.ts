import { apiGet, apiPatch, apiPost } from './client'
import type { ReviewItem } from '@/types/review'

export async function listReviewItems(params?: {
  status?: string
  limit?: number
  offset?: number
}): Promise<ReviewItem[]> {
  const p = new URLSearchParams()
  if (params?.status) p.set('status', params.status)
  if (params?.limit !== undefined) p.set('limit', String(params.limit))
  if (params?.offset !== undefined) p.set('offset', String(params.offset))
  const qs = p.toString()
  return apiGet<ReviewItem[]>(`/api/v1/review${qs ? `?${qs}` : ''}`)
}

export async function getReviewItem(id: string): Promise<ReviewItem> {
  return apiGet<ReviewItem>(`/api/v1/review/${encodeURIComponent(id)}`)
}

export async function updateReviewDraft(
  id: string,
  draft: Record<string, unknown>
): Promise<ReviewItem> {
  return apiPatch<ReviewItem>(`/api/v1/review/${encodeURIComponent(id)}`, { draft })
}

export async function approveReview(id: string): Promise<ReviewItem> {
  return apiPost<ReviewItem>(`/api/v1/review/${encodeURIComponent(id)}/approve`)
}

export async function rejectReview(id: string, reason: string): Promise<ReviewItem> {
  return apiPost<ReviewItem>(`/api/v1/review/${encodeURIComponent(id)}/reject`, { reason })
}
