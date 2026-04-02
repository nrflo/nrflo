import { apiGet } from './client'
import type { ErrorsResponse } from '@/types/errors'

export interface FetchErrorsParams {
  page?: number
  perPage?: number
  type?: string
}

export async function fetchErrors(
  params?: FetchErrorsParams
): Promise<ErrorsResponse> {
  const searchParams = new URLSearchParams()
  if (params?.page) searchParams.set('page', String(params.page))
  if (params?.perPage) searchParams.set('per_page', String(params.perPage))
  if (params?.type) searchParams.set('type', params.type)
  const query = searchParams.toString()
  return apiGet<ErrorsResponse>(`/api/v1/errors${query ? `?${query}` : ''}`)
}
