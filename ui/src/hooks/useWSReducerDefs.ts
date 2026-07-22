import type { QueryClient } from '@tanstack/react-query'
import type { WSEventV2 } from './useWSProtocol'
import { modelKeys } from './useModels'
import { customProviderKeys } from './useCustomProviders'
import type { WSEventType } from './useWebSocket'

export type WSEventHandler = (
  event: WSEventV2,
  qc: QueryClient,
  isProjectScope: boolean,
) => void

const invalidateDefs = (_event: WSEventV2, qc: QueryClient) => {
  qc.invalidateQueries({ queryKey: ['workflow-defs'] })
  qc.invalidateQueries({ queryKey: ['workflows', 'defs'] })
}

const invalidateWorkflowDefs = (event: WSEventV2, qc: QueryClient) => {
  invalidateDefs(event, qc)
  qc.invalidateQueries({ queryKey: ['workflow-layer-policies'] })
}

const invalidateAgentDefs = (event: WSEventV2, qc: QueryClient) => {
  invalidateDefs(event, qc)
  qc.invalidateQueries({ queryKey: ['agent-defs'] })
}

const invalidateModels = (_event: WSEventV2, qc: QueryClient) => {
  qc.invalidateQueries({ queryKey: modelKeys.list() })
}

const invalidateCustomProviders = (_event: WSEventV2, qc: QueryClient) => {
  qc.invalidateQueries({ queryKey: customProviderKeys.list() })
  qc.invalidateQueries({ queryKey: modelKeys.list() })
}

// Definition/registry events: global collections, no project/ticket scoping.
export const defRegistryHandlers: Partial<Record<WSEventType, WSEventHandler>> = {
  'workflow_def.created': invalidateWorkflowDefs,
  'workflow_def.updated': invalidateWorkflowDefs,
  'workflow_def.deleted': invalidateWorkflowDefs,
  'agent_def.created': invalidateAgentDefs,
  'agent_def.updated': invalidateAgentDefs,
  'agent_def.deleted': invalidateAgentDefs,
  'model.created': invalidateModels,
  'model.updated': invalidateModels,
  'model.deleted': invalidateModels,
  'custom_provider.created': invalidateCustomProviders,
  'custom_provider.updated': invalidateCustomProviders,
  'custom_provider.deleted': invalidateCustomProviders,
}
