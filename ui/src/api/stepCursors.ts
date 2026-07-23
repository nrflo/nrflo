import { apiGet } from './client'
import type { StepCursorsResponse } from '@/types/stepwise'

export async function fetchStepCursors(instanceId: string): Promise<StepCursorsResponse> {
  return apiGet<StepCursorsResponse>(
    `/api/v1/workflow-instances/${encodeURIComponent(instanceId)}/steps`
  )
}
