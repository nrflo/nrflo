import { describe, it, expect } from 'vitest'
import {
  PROTOCOL_VERSION,
  isV2Event,
  isControlEvent,
  subscriptionKey,
  type WSEventV2,
  type WSControlEventType,
  type WSSubscribeMessage,
} from './useWSProtocol'

describe('useWSProtocol', () => {
  describe('PROTOCOL_VERSION', () => {
    it('exports protocol version 2', () => {
      expect(PROTOCOL_VERSION).toBe(2)
    })
  })

  describe('isV2Event', () => {
    it('returns true for event with protocol_version=2 and sequence', () => {
      const event: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        protocol_version: 2,
        sequence: 42,
        timestamp: '2026-02-14T00:00:00Z',
      }
      expect(isV2Event(event)).toBe(true)
    })

    it('returns false when protocol_version is not 2', () => {
      const event: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        protocol_version: 1,
        sequence: 42,
        timestamp: '2026-02-14T00:00:00Z',
      }
      expect(isV2Event(event)).toBe(false)
    })

    it('returns false when sequence is undefined', () => {
      const event: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        protocol_version: 2,
        timestamp: '2026-02-14T00:00:00Z',
      }
      expect(isV2Event(event)).toBe(false)
    })

    it('returns false when both protocol_version and sequence are missing', () => {
      const event: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
      }
      expect(isV2Event(event)).toBe(false)
    })
  })

  describe('isControlEvent', () => {
    it('returns true for snapshot.begin', () => {
      expect(isControlEvent('snapshot.begin')).toBe(true)
    })

    it('returns true for snapshot.chunk', () => {
      expect(isControlEvent('snapshot.chunk')).toBe(true)
    })

    it('returns true for snapshot.end', () => {
      expect(isControlEvent('snapshot.end')).toBe(true)
    })

    it('returns true for resync.required', () => {
      expect(isControlEvent('resync.required')).toBe(true)
    })

    it('returns true for heartbeat', () => {
      expect(isControlEvent('heartbeat')).toBe(true)
    })

    it('returns false for workflow.updated', () => {
      expect(isControlEvent('workflow.updated')).toBe(false)
    })

    it('returns false for agent.started', () => {
      expect(isControlEvent('agent.started')).toBe(false)
    })

    it('returns false for unknown event type', () => {
      expect(isControlEvent('unknown.event')).toBe(false)
    })

    it('returns false for empty string', () => {
      expect(isControlEvent('')).toBe(false)
    })

    it('narrows type to WSControlEventType when true', () => {
      const eventType = 'snapshot.begin'
      if (isControlEvent(eventType)) {
        // Type should be narrowed to WSControlEventType
        const controlType: WSControlEventType = eventType
        expect(controlType).toBe('snapshot.begin')
      }
    })

    it('handles all control event types correctly', () => {
      const controlTypes: WSControlEventType[] = [
        'snapshot.begin',
        'snapshot.chunk',
        'snapshot.end',
        'resync.required',
        'heartbeat',
      ]

      controlTypes.forEach((type) => {
        expect(isControlEvent(type)).toBe(true)
      })
    })
  })

  describe('subscriptionKey', () => {
    it('creates key in format projectId:ticketId', () => {
      const key = subscriptionKey('proj-123', 'tick-456')
      expect(key).toBe('proj-123:tick-456')
    })

    it('handles empty ticket ID for project-scoped subscriptions', () => {
      const key = subscriptionKey('proj-123', '')
      expect(key).toBe('proj-123:')
    })

    it('handles special characters in IDs', () => {
      const key = subscriptionKey('proj-with-dash', 'TICKET-123')
      expect(key).toBe('proj-with-dash:TICKET-123')
    })

    it('creates consistent keys for same inputs', () => {
      const key1 = subscriptionKey('proj', 'ticket')
      const key2 = subscriptionKey('proj', 'ticket')
      expect(key1).toBe(key2)
    })

    it('creates different keys for different inputs', () => {
      const key1 = subscriptionKey('proj1', 'ticket1')
      const key2 = subscriptionKey('proj2', 'ticket2')
      expect(key1).not.toBe(key2)
    })
  })

  describe('WSEventV2 type', () => {
    it('accepts event with all v2 fields', () => {
      const event: WSEventV2 = {
        type: 'agent.started',
        project_id: 'proj1',
        ticket_id: 'tick1',
        workflow: 'feature',
        timestamp: '2026-02-14T00:00:00Z',
        protocol_version: 2,
        sequence: 100,
        entity: 'workflow_state',
        data: { foo: 'bar' },
      }
      expect(event.sequence).toBe(100)
      expect(event.entity).toBe('workflow_state')
    })

    it('accepts event with minimal required fields', () => {
      const event: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
      }
      expect(event.type).toBe('workflow.updated')
      expect(event.protocol_version).toBeUndefined()
      expect(event.sequence).toBeUndefined()
    })

    it('accepts control event types in type field', () => {
      const event: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        protocol_version: 2,
        sequence: 1,
      }
      expect(event.type).toBe('snapshot.begin')
    })
  })

  describe('SnapshotEntityType', () => {
    it('includes all expected entity types', () => {
      // This is a type test - we verify the entity values are accepted
      const entities = [
        'workflow_state',
        'agent_sessions',
        'findings',
        'ticket_detail',
        'chain_status',
      ]

      entities.forEach((entity) => {
        const event: WSEventV2 = {
          type: 'snapshot.chunk',
          project_id: 'proj1',
          ticket_id: 'tick1',
          timestamp: '2026-02-14T00:00:00Z',
          entity: entity as any,
          data: {},
        }
        expect(event.entity).toBe(entity)
      })
    })
  })

  describe('WSSubscribeMessage type', () => {
    it('accepts subscribe message with cursor', () => {
      const msg = {
        action: 'subscribe' as const,
        project_id: 'proj1',
        ticket_id: 'tick1',
        since_seq: 42,
      }
      expect(msg.since_seq).toBe(42)
    })

    it('accepts subscribe message without cursor', () => {
      const msg: WSSubscribeMessage = {
        action: 'subscribe',
        project_id: 'proj1',
        ticket_id: 'tick1',
      }
      expect(msg.since_seq).toBeUndefined()
    })

    it('accepts unsubscribe message', () => {
      const msg = {
        action: 'unsubscribe' as const,
        project_id: 'proj1',
        ticket_id: 'tick1',
      }
      expect(msg.action).toBe('unsubscribe')
    })
  })
})
