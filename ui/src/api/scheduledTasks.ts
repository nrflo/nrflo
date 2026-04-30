import { apiGet, apiPost, apiPatch, apiDelete } from './client'
import type {
  ScheduledTask,
  ScheduleRun,
  ScheduledTaskCreateRequest,
  ScheduledTaskUpdateRequest,
} from '@/types/schedules'

export interface ListScheduleRunsParams {
  limit?: number
  offset?: number
}

export async function listScheduledTasks(): Promise<ScheduledTask[]> {
  return apiGet<ScheduledTask[]>('/api/v1/scheduled-tasks')
}

export async function getScheduledTask(id: string): Promise<ScheduledTask> {
  return apiGet<ScheduledTask>(`/api/v1/scheduled-tasks/${encodeURIComponent(id)}`)
}

export async function createScheduledTask(data: ScheduledTaskCreateRequest): Promise<ScheduledTask> {
  return apiPost<ScheduledTask>('/api/v1/scheduled-tasks', data)
}

export async function updateScheduledTask(
  id: string,
  data: ScheduledTaskUpdateRequest
): Promise<ScheduledTask> {
  return apiPatch<ScheduledTask>(`/api/v1/scheduled-tasks/${encodeURIComponent(id)}`, data)
}

export async function deleteScheduledTask(id: string): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(`/api/v1/scheduled-tasks/${encodeURIComponent(id)}`)
}

export async function listScheduleRuns(
  taskId: string,
  params?: ListScheduleRunsParams
): Promise<ScheduleRun[]> {
  const searchParams = new URLSearchParams()
  if (params?.limit !== undefined) searchParams.set('limit', String(params.limit))
  if (params?.offset !== undefined) searchParams.set('offset', String(params.offset))
  const query = searchParams.toString()
  return apiGet<ScheduleRun[]>(
    `/api/v1/scheduled-tasks/${encodeURIComponent(taskId)}/runs${query ? `?${query}` : ''}`
  )
}

export async function runScheduledTaskNow(id: string): Promise<{ status: string }> {
  return apiPost<{ status: string }>(
    `/api/v1/scheduled-tasks/${encodeURIComponent(id)}/run-now`,
    {}
  )
}
