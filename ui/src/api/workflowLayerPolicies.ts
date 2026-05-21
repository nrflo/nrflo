import { apiGet, apiPut, apiDelete } from './client'
import type { LayerPassPolicy, LayerPoliciesResponse } from '@/types/workflow'

/** List all layer policies and pause-after flags for a workflow */
export async function listLayerPolicies(workflowId: string): Promise<LayerPoliciesResponse> {
  return apiGet<LayerPoliciesResponse>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/layer-policies`
  )
}

/** Set (or update) the pass policy and optional pause-after flag for a specific layer */
export async function setLayerPolicy(
  workflowId: string,
  layer: number,
  pass_policy: LayerPassPolicy,
  pause_after?: boolean
): Promise<{ status: string }> {
  return apiPut<{ status: string }>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/layer-policies/${layer}`,
    pause_after !== undefined ? { pass_policy, pause_after } : { pass_policy }
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
