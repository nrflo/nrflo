import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useProjectStore } from '@/stores/projectStore'
import { getPlan, listPlanRevisions, revisePlan, approvePlan, cancelPlan, startDynamicWorkflow } from '@/api/plan'
import { projectWorkflowKeys } from './useTickets'
import type { PlanApproveRequest, PlanReviseRequest, DynamicWorkflowRunRequest } from '@/types/plan'

export const planKeys = {
  all: ['plan'] as const,
  detail: (iid: string) => [...planKeys.all, iid] as const,
  revisions: (iid: string) => [...planKeys.detail(iid), 'revisions'] as const,
}

export function usePlan(instanceId: string | undefined) {
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: planKeys.detail(instanceId ?? ''),
    queryFn: () => getPlan(instanceId!),
    enabled: projectsLoaded && !!instanceId,
  })
}

export function usePlanRevisions(instanceId: string | undefined) {
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: planKeys.revisions(instanceId ?? ''),
    queryFn: () => listPlanRevisions(instanceId!),
    enabled: projectsLoaded && !!instanceId,
  })
}

export function useRevisePlan() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ instanceId, params }: { instanceId: string; params: PlanReviseRequest }) =>
      revisePlan(instanceId, params),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: planKeys.detail(variables.instanceId) })
    },
  })
}

export function useApprovePlan() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ instanceId, params }: { instanceId: string; params: PlanApproveRequest }) =>
      approvePlan(instanceId, params),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: planKeys.detail(variables.instanceId) })
    },
  })
}

export function useCancelPlan() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ instanceId }: { instanceId: string }) => cancelPlan(instanceId),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: planKeys.detail(variables.instanceId) })
    },
  })
}

export function useStartDynamicWorkflow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, params }: { projectId: string; params: DynamicWorkflowRunRequest }) =>
      startDynamicWorkflow(projectId, params),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(variables.projectId) })
    },
  })
}
