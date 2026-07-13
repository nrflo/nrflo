import { apiGet, apiPost } from './client'
import type { PlanDraft, PlanRevision, PlanReviseRequest, PlanApproveRequest } from '@/types/plan'

function planBase(iid: string): string {
  return `/api/v1/workflow-instances/${encodeURIComponent(iid)}/plan`
}

export function getPlan(iid: string): Promise<PlanDraft> {
  return apiGet<PlanDraft>(planBase(iid))
}

export function listPlanRevisions(iid: string): Promise<{ revisions: PlanRevision[] }> {
  return apiGet<{ revisions: PlanRevision[] }>(`${planBase(iid)}/revisions`)
}

export function revisePlan(iid: string, req: PlanReviseRequest): Promise<PlanRevision> {
  return apiPost<PlanRevision>(`${planBase(iid)}/revise`, req)
}

export function approvePlan(iid: string, req: PlanApproveRequest): Promise<PlanRevision> {
  return apiPost<PlanRevision>(`${planBase(iid)}/approve`, req)
}

export function cancelPlan(iid: string): Promise<{ status: string }> {
  return apiPost<{ status: string }>(`${planBase(iid)}/cancel`)
}
