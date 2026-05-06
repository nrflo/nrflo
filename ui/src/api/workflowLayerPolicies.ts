import { apiGet, apiPut, apiDelete } from './client'
import type { LayerPassPolicy } from '@/types/workflow'

/** List all layer policies for a workflow (keyed by layer number) */
export async function listLayerPolicies(workflowId: string): Promise<Record<number, LayerPassPolicy>> {
  return apiGet<Record<number, LayerPassPolicy>>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/layer-policies`
  )
}

/** Set (or update) the pass policy for a specific layer */
export async function setLayerPolicy(
  workflowId: string,
  layer: number,
  pass_policy: LayerPassPolicy
): Promise<{ status: string }> {
  return apiPut<{ status: string }>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/layer-policies/${layer}`,
    { pass_policy }
  )
}

/** Delete (reset to default 'any') the pass policy for a specific layer */
export async function deleteLayerPolicy(
  workflowId: string,
  layer: number
): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/layer-policies/${layer}`
  )
}
