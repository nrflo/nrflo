import { apiGet, apiPost, apiPatch, apiDelete } from './client'
import type {
  Ticket,
  TicketWithDeps,
  CreateTicketRequest,
  UpdateTicketRequest,
  TicketListResponse,
  SearchResponse,
  StatusResponse,
  DailyStats,
} from '@/types/ticket'
import type {
  WorkflowResponse,
  UpdateWorkflowRequest,
  DependenciesResponse,
  DependencyRequest,
  AgentSessionsResponse,
  SessionMessagesResponse,
} from '@/types/workflow'

export interface ListTicketsParams {
  status?: string
  type?: string
}

export async function listTickets(
  params?: ListTicketsParams
): Promise<TicketListResponse> {
  const searchParams = new URLSearchParams()
  if (params?.status) searchParams.set('status', params.status)
  if (params?.type) searchParams.set('type', params.type)
  const query = searchParams.toString()
  return apiGet<TicketListResponse>(`/api/v1/tickets${query ? `?${query}` : ''}`)
}

export async function getTicket(id: string): Promise<TicketWithDeps> {
  return apiGet<TicketWithDeps>(`/api/v1/tickets/${encodeURIComponent(id)}`)
}

export async function createTicket(data: CreateTicketRequest): Promise<Ticket> {
  return apiPost<Ticket>('/api/v1/tickets', data)
}

export async function updateTicket(
  id: string,
  data: UpdateTicketRequest
): Promise<Ticket> {
  return apiPatch<Ticket>(`/api/v1/tickets/${encodeURIComponent(id)}`, data)
}

export async function closeTicket(
  id: string,
  reason?: string
): Promise<Ticket> {
  return apiPost<Ticket>(`/api/v1/tickets/${encodeURIComponent(id)}/close`, {
    reason,
  })
}

export async function reopenTicket(id: string): Promise<Ticket> {
  return apiPost<Ticket>(
    `/api/v1/tickets/${encodeURIComponent(id)}/reopen`,
    {}
  )
}

export async function deleteTicket(
  id: string
): Promise<{ message: string }> {
  return apiDelete<{ message: string }>(
    `/api/v1/tickets/${encodeURIComponent(id)}`
  )
}

export async function searchTickets(query: string): Promise<SearchResponse> {
  return apiGet<SearchResponse>(
    `/api/v1/search?q=${encodeURIComponent(query)}`
  )
}

export async function getStatus(limit?: number): Promise<StatusResponse> {
  const query = limit ? `?limit=${limit}` : ''
  return apiGet<StatusResponse>(`/api/v1/status${query}`)
}

export async function getDailyStats(): Promise<DailyStats> {
  return apiGet<DailyStats>('/api/v1/daily-stats')
}

// Workflow endpoints
export async function getWorkflow(ticketId: string): Promise<WorkflowResponse> {
  return apiGet<WorkflowResponse>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow`
  )
}

export async function updateWorkflow(
  ticketId: string,
  data: UpdateWorkflowRequest
): Promise<WorkflowResponse> {
  return apiPatch<WorkflowResponse>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow`,
    data
  )
}

// Dependencies endpoints
export async function getDependencies(
  ticketId: string
): Promise<DependenciesResponse> {
  return apiGet<DependenciesResponse>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/dependencies`
  )
}

export async function addDependency(
  data: DependencyRequest
): Promise<{ message: string; child_id: string; parent_id: string }> {
  return apiPost('/api/v1/dependencies', data)
}

export async function removeDependency(
  data: DependencyRequest
): Promise<{ message: string; child_id: string; parent_id: string }> {
  return apiDelete('/api/v1/dependencies', data)
}

// Agent sessions endpoints
export async function getAgentSessions(
  ticketId: string,
  phase?: string
): Promise<AgentSessionsResponse> {
  const params = phase ? `?phase=${encodeURIComponent(phase)}` : ''
  return apiGet<AgentSessionsResponse>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/agents${params}`
  )
}

// Session messages (lazy-loaded)
export async function getSessionMessages(
  sessionId: string,
  category?: string
): Promise<SessionMessagesResponse> {
  const params = category ? `?category=${encodeURIComponent(category)}` : ''
  return apiGet<SessionMessagesResponse>(
    `/api/v1/sessions/${encodeURIComponent(sessionId)}/messages${params}`
  )
}

