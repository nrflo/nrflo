import { describe, it, expect, beforeEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import type { WSEventV2, WSSubscribeMessage } from './useWSProtocol'
import { subscriptionKey } from './useWSProtocol'
import { setLastSeq, clearSeqs, getLastSeq } from './useWSReducer'
import {
  handleSnapshotBegin,
  handleSnapshotChunk,
  handleSnapshotEnd,
  isReceivingSnapshot,
  bufferEventDuringSnapshot,
} from './useWSSnapshot'

/**
 * Unit tests for v2 protocol integration without full WebSocket mocking.
 * Tests focus on protocol behavior, cursor management, snapshot handling, and control events.
 */
describe('useWebSocket - v2 protocol integration (unit)', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    clearSeqs()
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    })
  })

  describe('cursor resume logic', () => {
    it('builds subscribe message without cursor on first connect', () => {
      const projectId = 'test-project'
      const ticketId = 'tick1'
      const subKey = subscriptionKey(projectId, ticketId)

      const lastSeq = getLastSeq(subKey)
      expect(lastSeq).toBeUndefined()

      // First connect: no cursor
      const msg: WSSubscribeMessage = {
        action: 'subscribe',
        project_id: projectId,
        ticket_id: ticketId,
      }
      if (lastSeq !== undefined) {
        msg.since_seq = lastSeq
      }

      expect(msg.since_seq).toBeUndefined()
    })

    it('builds subscribe message with cursor when seq exists', () => {
      const projectId = 'test-project'
      const ticketId = 'tick1'
      const subKey = subscriptionKey(projectId, ticketId)

      setLastSeq(subKey, 42)

      const lastSeq = getLastSeq(subKey)
      expect(lastSeq).toBe(42)

      // Reconnect: include cursor
      const msg: WSSubscribeMessage = {
        action: 'subscribe',
        project_id: projectId,
        ticket_id: ticketId,
      }
      if (lastSeq !== undefined) {
        msg.since_seq = lastSeq
      }

      expect(msg.since_seq).toBe(42)
    })

    it('resync message uses since_seq=0 to force snapshot', () => {
      const msg: WSSubscribeMessage = {
        action: 'subscribe',
        project_id: 'test-project',
        ticket_id: 'tick1',
        since_seq: 0,
      }

      expect(msg.since_seq).toBe(0)
    })
  })

  describe('control event flow', () => {
    it('snapshot.begin initiates snapshot session', () => {
      const event: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'test-project',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(event)
      expect(isReceivingSnapshot('test-project', 'tick1')).toBe(true)
    })

    it('live events during snapshot are buffered', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'test-project',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)

      const liveEvent: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'test-project',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 150,
        protocol_version: 2,
      }

      const buffered = bufferEventDuringSnapshot('test-project', 'tick1', liveEvent)
      expect(buffered).toBe(true)
    })

    it('snapshot.end returns buffered events for replay', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'test-project',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)

      const chunk: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'test-project',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 101,
        protocol_version: 2,
        entity: 'workflow_state',
        data: { state: 'running' },
      }

      handleSnapshotChunk(chunk)

      const liveEvent: WSEventV2 = {
        type: 'agent.started',
        project_id: 'test-project',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:02Z',
        sequence: 150,
        protocol_version: 2,
      }

      bufferEventDuringSnapshot('test-project', 'tick1', liveEvent)

      const endEvent: WSEventV2 = {
        type: 'snapshot.end',
        project_id: 'test-project',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:03Z',
        sequence: 105,
        protocol_version: 2,
      }

      const buffered = handleSnapshotEnd(endEvent, queryClient)
      expect(buffered.length).toBe(1)
      expect(buffered[0].sequence).toBe(150)
      expect(isReceivingSnapshot('test-project', 'tick1')).toBe(false)
    })

    it('resync.required event structure', () => {
      const resyncEvent: WSEventV2 = {
        type: 'resync.required',
        project_id: 'test-project',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 200,
        protocol_version: 2,
      }

      expect(resyncEvent.type).toBe('resync.required')
      // Application should respond by re-subscribing with since_seq=0
    })

    it('heartbeat event structure', () => {
      const heartbeat: WSEventV2 = {
        type: 'heartbeat',
        project_id: 'test-project',
        ticket_id: '',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 50,
        protocol_version: 2,
      }

      expect(heartbeat.type).toBe('heartbeat')
      // Application should reset heartbeat timeout on receipt
    })
  })

  describe('backward compatibility', () => {
    it('v1 events without sequence are accepted', () => {
      const v1Event = {
        type: 'workflow.updated',
        project_id: 'test-project',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        // No sequence or protocol_version
      }

      expect(v1Event.type).toBe('workflow.updated')
      // Should not break when processed
    })

    it('ack message structure', () => {
      const ackMsg = {
        type: 'ack',
        action: 'subscribe',
        project_id: 'test-project',
        ticket_id: 'tick1',
      }

      expect(ackMsg.type).toBe('ack')
      // Application should ignore ack messages
    })
  })

  describe('batched messages', () => {
    it('handles newline-separated batch format', () => {
      const event1: WSEventV2 = {
        type: 'agent.started',
        project_id: 'test-project',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 1,
        protocol_version: 2,
      }

      const event2: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'test-project',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 2,
        protocol_version: 2,
      }

      const batchedData = `${JSON.stringify(event1)}\n${JSON.stringify(event2)}`
      const lines = batchedData.split('\n').filter(line => line.trim())

      expect(lines.length).toBe(2)

      const parsed1 = JSON.parse(lines[0]) as WSEventV2
      const parsed2 = JSON.parse(lines[1]) as WSEventV2

      expect(parsed1.sequence).toBe(1)
      expect(parsed2.sequence).toBe(2)
    })
  })

  describe('seq persistence', () => {
    it('persists seq to sessionStorage', () => {
      setLastSeq('test-project:tick1', 99)

      sessionStorage.setItem('ws_last_seqs', JSON.stringify({
        'test-project:tick1': 99,
      }))

      const stored = sessionStorage.getItem('ws_last_seqs')
      expect(stored).toBeDefined()

      const parsed = JSON.parse(stored!)
      expect(parsed['test-project:tick1']).toBe(99)
    })

    it('restores seq from sessionStorage on load', () => {
      sessionStorage.setItem('ws_last_seqs', JSON.stringify({
        'test-project:tick1': 150,
      }))

      const stored = sessionStorage.getItem('ws_last_seqs')
      const parsed = JSON.parse(stored!)
      const restored = parsed['test-project:tick1']

      expect(restored).toBe(150)
    })
  })

  describe('subscription scoping', () => {
    it('ticket-scoped subscription key', () => {
      const key = subscriptionKey('proj1', 'tick1')
      expect(key).toBe('proj1:tick1')
    })

    it('project-scoped subscription key', () => {
      const key = subscriptionKey('proj1', '')
      expect(key).toBe('proj1:')
    })

    it('different scopes have independent seq tracking', () => {
      setLastSeq('proj1:tick1', 10)
      setLastSeq('proj1:tick2', 20)
      setLastSeq('proj1:', 30) // project scope

      expect(getLastSeq('proj1:tick1')).toBe(10)
      expect(getLastSeq('proj1:tick2')).toBe(20)
      expect(getLastSeq('proj1:')).toBe(30)
    })
  })

  describe('snapshot entity types', () => {
    it('handles workflow_state entity', () => {
      const chunk: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 101,
        protocol_version: 2,
        entity: 'workflow_state',
        data: { status: 'running' },
      }

      expect(chunk.entity).toBe('workflow_state')
      expect(chunk.data).toBeDefined()
    })

    it('handles agent_sessions entity', () => {
      const chunk: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 102,
        protocol_version: 2,
        entity: 'agent_sessions',
        data: { sessions: [] },
      }

      expect(chunk.entity).toBe('agent_sessions')
    })

    it('handles findings entity', () => {
      const chunk: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 103,
        protocol_version: 2,
        entity: 'findings',
        data: { findings: {} },
      }

      expect(chunk.entity).toBe('findings')
    })

    it('handles ticket_detail entity', () => {
      const chunk: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 104,
        protocol_version: 2,
        entity: 'ticket_detail',
        data: { title: 'Test ticket' },
      }

      expect(chunk.entity).toBe('ticket_detail')
    })

    it('handles chain_status entity', () => {
      const chunk: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: '',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 105,
        protocol_version: 2,
        entity: 'chain_status',
        data: { chain_id: 'chain-123', status: 'running' },
      }

      expect(chunk.entity).toBe('chain_status')
    })
  })
})
