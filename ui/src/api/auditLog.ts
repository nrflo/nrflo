import { apiGet } from './client'
import type { AuditListResponse } from '@/types/audit'

export interface AuditLogParams {
  page?: number
  per_page?: number
  user_id?: string
  action?: string
}

export async function listAuditLog(params?: AuditLogParams): Promise<AuditListResponse> {
  const searchParams = new URLSearchParams()
  if (params?.page !== undefined) searchParams.set('page', String(params.page))
  if (params?.per_page !== undefined) searchParams.set('per_page', String(params.per_page))
  if (params?.user_id) searchParams.set('user_id', params.user_id)
  if (params?.action) searchParams.set('action', params.action)
  const query = searchParams.toString()
  return apiGet<AuditListResponse>(`/api/v1/audit-log${query ? `?${query}` : ''}`)
}
