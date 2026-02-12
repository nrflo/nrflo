import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'
import { runWorkflow, stopWorkflow, restartAgent } from '@/api/workflows'
import {
  getProjectWorkflow,
  runProjectWorkflow,
  stopProjectWorkflow,
  restartProjectAgent,
} from '@/api/projectWorkflows'
import {
  listTickets,
  getTicket,
  createTicket,
  updateTicket,
  closeTicket,
  reopenTicket,
  deleteTicket,
  searchTickets,
  getStatus,
  getWorkflow,
  updateWorkflow,
  getAgentSessions,
  getSessionMessages,
  type ListTicketsParams,
} from '@/api/tickets'
import type {
  Ticket,
  TicketWithDeps,
  CreateTicketRequest,
  UpdateTicketRequest,
  TicketListResponse,
  StatusResponse,
} from '@/types/ticket'
import type { WorkflowResponse, ProjectWorkflowResponse, UpdateWorkflowRequest, AgentSessionsResponse, RunWorkflowRequest, ProjectWorkflowRunRequest, RestartAgentRequest, SessionMessagesResponse } from '@/types/workflow'
import { useProjectStore } from '@/stores/projectStore'

// Query keys factory
export const ticketKeys = {
  all: ['tickets'] as const,
  lists: () => [...ticketKeys.all, 'list'] as const,
  list: (params?: ListTicketsParams) =>
    [...ticketKeys.lists(), params] as const,
  details: () => [...ticketKeys.all, 'detail'] as const,
  detail: (id: string) => [...ticketKeys.details(), id] as const,
  workflow: (id: string) => [...ticketKeys.detail(id), 'workflow'] as const,
  agentSessions: (id: string, phase?: string) =>
    [...ticketKeys.detail(id), 'agents', phase] as const,
  search: (query: string) => [...ticketKeys.all, 'search', query] as const,
  status: () => [...ticketKeys.all, 'status'] as const,
}

export function useTicketList(
  params?: ListTicketsParams,
  options?: Omit<UseQueryOptions<TicketListResponse>, 'queryKey' | 'queryFn'>
) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...ticketKeys.list(params), project],
    queryFn: () => listTickets(params),
    enabled: projectsLoaded && (options?.enabled ?? true),
    ...options,
  })
}

export function useTicket(
  id: string,
  options?: Omit<UseQueryOptions<TicketWithDeps>, 'queryKey' | 'queryFn'>
) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...ticketKeys.detail(id), project],
    queryFn: () => getTicket(id),
    enabled: projectsLoaded && !!id && (options?.enabled ?? true),
    ...options,
  })
}

export function useWorkflow(
  ticketId: string,
  options?: Omit<UseQueryOptions<WorkflowResponse>, 'queryKey' | 'queryFn'>
) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...ticketKeys.workflow(ticketId), project],
    queryFn: () => getWorkflow(ticketId),
    enabled: projectsLoaded && !!ticketId && (options?.enabled ?? true),
    ...options,
  })
}

export function useTicketSearch(
  query: string,
  options?: Omit<UseQueryOptions<TicketListResponse>, 'queryKey' | 'queryFn'>
) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...ticketKeys.search(query), project],
    queryFn: async () => {
      const result = await searchTickets(query)
      return { tickets: result.tickets }
    },
    enabled: projectsLoaded && query.length >= 2 && (options?.enabled ?? true),
    ...options,
  })
}

export function useStatus(
  options?: Omit<UseQueryOptions<StatusResponse>, 'queryKey' | 'queryFn'>
) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...ticketKeys.status(), project],
    queryFn: () => getStatus(),
    enabled: projectsLoaded && (options?.enabled ?? true),
    ...options,
  })
}

export function useCreateTicket() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateTicketRequest) => createTicket(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
      queryClient.invalidateQueries({ queryKey: ticketKeys.status() })
    },
  })
}

export function useUpdateTicket() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateTicketRequest }) =>
      updateTicket(id, data),
    onSuccess: (ticket: Ticket) => {
      queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
      queryClient.invalidateQueries({ queryKey: ticketKeys.detail(ticket.id) })
      queryClient.invalidateQueries({ queryKey: ticketKeys.status() })
    },
  })
}

export function useCloseTicket() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason?: string }) =>
      closeTicket(id, reason),
    onSuccess: (ticket: Ticket) => {
      queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
      queryClient.invalidateQueries({ queryKey: ticketKeys.detail(ticket.id) })
      queryClient.invalidateQueries({ queryKey: ticketKeys.status() })
    },
  })
}

export function useReopenTicket() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id }: { id: string }) => reopenTicket(id),
    onSuccess: (ticket: Ticket) => {
      queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
      queryClient.invalidateQueries({ queryKey: ticketKeys.detail(ticket.id) })
      queryClient.invalidateQueries({ queryKey: ticketKeys.status() })
    },
  })
}

export function useDeleteTicket() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteTicket(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
      queryClient.invalidateQueries({ queryKey: ticketKeys.status() })
    },
  })
}

export function useUpdateWorkflow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      ticketId,
      data,
    }: {
      ticketId: string
      data: UpdateWorkflowRequest
    }) => updateWorkflow(ticketId, data),
    onSuccess: (response: WorkflowResponse) => {
      queryClient.invalidateQueries({
        queryKey: ticketKeys.workflow(response.ticket_id),
      })
      queryClient.invalidateQueries({
        queryKey: ticketKeys.detail(response.ticket_id),
      })
    },
  })
}

export function useAgentSessions(
  ticketId: string,
  phase?: string,
  options?: Omit<UseQueryOptions<AgentSessionsResponse>, 'queryKey' | 'queryFn'> & {
    refetchInterval?: number | false
  }
) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...ticketKeys.agentSessions(ticketId, phase), project],
    queryFn: () => getAgentSessions(ticketId, phase),
    enabled: projectsLoaded && !!ticketId && (options?.enabled ?? true),
    refetchInterval: options?.refetchInterval,
    ...options,
  })
}

export function useRunWorkflow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ ticketId, params }: { ticketId: string; params: RunWorkflowRequest }) =>
      runWorkflow(ticketId, params),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(variables.ticketId) })
    },
  })
}

export function useStopWorkflow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ ticketId, workflow }: { ticketId: string; workflow?: string }) =>
      stopWorkflow(ticketId, workflow ? { workflow } : undefined),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(variables.ticketId) })
    },
  })
}

export function useRestartAgent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ ticketId, params }: { ticketId: string; params: RestartAgentRequest }) =>
      restartAgent(ticketId, params),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ticketKeys.workflow(variables.ticketId) })
      queryClient.invalidateQueries({ queryKey: ticketKeys.agentSessions(variables.ticketId) })
    },
  })
}

export function useSessionMessages(
  sessionId: string | undefined,
  options?: { enabled?: boolean; isRunning?: boolean }
) {
  return useQuery<SessionMessagesResponse>({
    queryKey: ['session-messages', sessionId],
    queryFn: () => getSessionMessages(sessionId!),
    enabled: !!sessionId && (options?.enabled ?? true),
    staleTime: options?.isRunning ? 2000 : 30000,
    refetchInterval: options?.isRunning ? 3000 : false,
  })
}

// --- Project workflow hooks ---

export const projectWorkflowKeys = {
  all: ['project-workflows'] as const,
  workflow: (projectId: string) => [...projectWorkflowKeys.all, projectId] as const,
}

export function useProjectWorkflow(
  projectId: string,
  options?: Omit<UseQueryOptions<ProjectWorkflowResponse>, 'queryKey' | 'queryFn'>
) {
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: projectWorkflowKeys.workflow(projectId),
    queryFn: () => getProjectWorkflow(projectId),
    enabled: projectsLoaded && !!projectId && (options?.enabled ?? true),
    ...options,
  })
}

export function useRunProjectWorkflow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, params }: { projectId: string; params: ProjectWorkflowRunRequest }) =>
      runProjectWorkflow(projectId, params),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(variables.projectId) })
    },
  })
}

export function useStopProjectWorkflow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, workflow }: { projectId: string; workflow?: string }) =>
      stopProjectWorkflow(projectId, workflow),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(variables.projectId) })
    },
  })
}

export function useRestartProjectAgent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, params }: { projectId: string; params: RestartAgentRequest }) =>
      restartProjectAgent(projectId, params),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: projectWorkflowKeys.workflow(variables.projectId) })
    },
  })
}

