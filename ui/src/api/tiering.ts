import { apiGet, apiPost } from './client'
import type { TieringApplyRequest, TieringApplyResult, TieringReport } from '@/types/tiering'

export const getTieringReport = () => apiGet<TieringReport>('/api/v1/admin/tiering-report')

export const applyTiering = (payload: TieringApplyRequest) =>
  apiPost<TieringApplyResult>('/api/v1/admin/tiering-apply', payload)
