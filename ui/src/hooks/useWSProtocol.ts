import type { WSEventType } from './useWebSocket'

// Protocol v2 constants — must match be/internal/ws/protocol.go
export const PROTOCOL_VERSION = 2

// Control event types sent by server
export type WSControlEventType =
  | 'snapshot.begin'
  | 'snapshot.chunk'
  | 'snapshot.end'
  | 'resync.required'
  | 'heartbeat'

// Entity types used in snapshot chunks — must match backend constants
export type SnapshotEntityType =
  | 'workflow_state'
  | 'agent_sessions'
  | 'findings'
  | 'ticket_detail'
  | 'chain_status'

// Extended event with v2 fields (seq + protocol_version)
export interface WSEventV2 {
  type: WSEventType | WSControlEventType
  project_id: string
  ticket_id: string
  workflow?: string
  timestamp: string
  protocol_version?: number
  sequence?: number
  entity?: SnapshotEntityType
  data?: Record<string, unknown>
}

// Subscribe message with optional cursor for v2 replay
export interface WSSubscribeMessage {
  action: 'subscribe' | 'unsubscribe'
  project_id: string
  ticket_id: string
  since_seq?: number
}

// Check if event is protocol v2 (has sequence number)
export function isV2Event(event: WSEventV2): boolean {
  return event.protocol_version === PROTOCOL_VERSION && event.sequence !== undefined
}

// Check if event is a control event
export function isControlEvent(type: string): type is WSControlEventType {
  return (
    type === 'snapshot.begin' ||
    type === 'snapshot.chunk' ||
    type === 'snapshot.end' ||
    type === 'resync.required' ||
    type === 'heartbeat'
  )
}

// Subscription key for seq tracking (matches backend format)
export function subscriptionKey(projectId: string, ticketId: string): string {
  return `${projectId}:${ticketId}`
}
