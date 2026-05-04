import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listChains,
  getChain,
  createChain,
  updateChain,
  deleteChain,
  appendStep,
  updateStep,
  deleteStep,
  reorderSteps,
} from '@/api/workflowChains'
import type {
  WorkflowChainCreateRequest,
  WorkflowChainUpdateRequest,
  WorkflowChainStepRequest,
  WorkflowChainStepUpdateRequest,
  ReorderStepsRequest,
} from '@/types/workflowChain'
import { useProjectStore } from '@/stores/projectStore'

export const workflowChainKeys = {
  all: ['workflow-chains'] as const,
  lists: (project: string) => [...workflowChainKeys.all, 'list', project] as const,
  detail: (project: string, id: string) => [...workflowChainKeys.all, project, id] as const,
}

export function useWorkflowChainsList() {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: workflowChainKeys.lists(project),
    queryFn: listChains,
    enabled: projectsLoaded,
  })
}

export function useWorkflowChain(id: string) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: workflowChainKeys.detail(project, id),
    queryFn: () => getChain(id),
    enabled: projectsLoaded && !!id,
  })
}

export function useCreateWorkflowChain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: WorkflowChainCreateRequest) => createChain(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: workflowChainKeys.all })
    },
  })
}

export function useUpdateWorkflowChain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: WorkflowChainUpdateRequest }) =>
      updateChain(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: workflowChainKeys.all })
    },
  })
}

export function useDeleteWorkflowChain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteChain(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: workflowChainKeys.all })
    },
  })
}

export function useAppendStep() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ chainId, data }: { chainId: string; data: WorkflowChainStepRequest }) =>
      appendStep(chainId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: workflowChainKeys.all })
    },
  })
}

export function useUpdateStep() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      chainId,
      stepId,
      data,
    }: {
      chainId: string
      stepId: string
      data: WorkflowChainStepUpdateRequest
    }) => updateStep(chainId, stepId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: workflowChainKeys.all })
    },
  })
}

export function useDeleteStep() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ chainId, stepId }: { chainId: string; stepId: string }) =>
      deleteStep(chainId, stepId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: workflowChainKeys.all })
    },
  })
}

export function useReorderSteps() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ chainId, data }: { chainId: string; data: ReorderStepsRequest }) =>
      reorderSteps(chainId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: workflowChainKeys.all })
    },
  })
}
