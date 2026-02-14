import { describe, it, expect, beforeEach, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import {
  isReceivingSnapshot,
  handleSnapshotBegin,
  handleSnapshotChunk,
  handleSnapshotEnd,
  bufferEventDuringSnapshot,
  drainBufferedEvents,
} from './useWSSnapshot'
import type { WSEventV2, SnapshotEntityType } from './useWSProtocol'

describe('useWSSnapshot', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    })
  })

  describe('snapshot state machine', () => {
    it('isReceivingSnapshot returns false initially', () => {
      expect(isReceivingSnapshot('proj1', 'tick1')).toBe(false)
    })

    it('handleSnapshotBegin starts snapshot session', () => {
      const event: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(event)
      expect(isReceivingSnapshot('proj1', 'tick1')).toBe(true)
    })

    it('handleSnapshotEnd completes snapshot session', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      const endEvent: WSEventV2 = {
        type: 'snapshot.end',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:02Z',
        sequence: 105,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)
      expect(isReceivingSnapshot('proj1', 'tick1')).toBe(true)

      handleSnapshotEnd(endEvent, queryClient)
      expect(isReceivingSnapshot('proj1', 'tick1')).toBe(false)
    })

    it('multiple subscriptions tracked independently', () => {
      const event1: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      const event2: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick2',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 200,
        protocol_version: 2,
      }

      handleSnapshotBegin(event1)
      handleSnapshotBegin(event2)

      expect(isReceivingSnapshot('proj1', 'tick1')).toBe(true)
      expect(isReceivingSnapshot('proj1', 'tick2')).toBe(true)
      expect(isReceivingSnapshot('proj1', 'tick3')).toBe(false)
    })
  })

  describe('snapshot chunk accumulation', () => {
    it('handleSnapshotChunk accumulates chunks by entity type', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      const chunk1: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 101,
        protocol_version: 2,
        entity: 'workflow_state',
        data: { state: 'running' },
      }

      const chunk2: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:02Z',
        sequence: 102,
        protocol_version: 2,
        entity: 'agent_sessions',
        data: { sessions: [] },
      }

      handleSnapshotBegin(beginEvent)
      handleSnapshotChunk(chunk1)
      handleSnapshotChunk(chunk2)

      // Chunks are accumulated — verification happens on end
      const endEvent: WSEventV2 = {
        type: 'snapshot.end',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:03Z',
        sequence: 103,
        protocol_version: 2,
      }

      vi.spyOn(queryClient, 'invalidateQueries')
      handleSnapshotEnd(endEvent, queryClient)

      // Should have invalidated queries for workflow_state and agent_sessions
      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })

    it('handleSnapshotChunk ignores chunks when not receiving', () => {
      const chunk: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 101,
        protocol_version: 2,
        entity: 'workflow_state',
        data: { state: 'running' },
      }

      // No snapshot.begin called
      expect(() => handleSnapshotChunk(chunk)).not.toThrow()
    })

    it('handleSnapshotChunk tracks highest seq', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      const chunk1: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 105,
        protocol_version: 2,
        entity: 'workflow_state',
        data: {},
      }

      const chunk2: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:02Z',
        sequence: 110,
        protocol_version: 2,
        entity: 'findings',
        data: {},
      }

      handleSnapshotBegin(beginEvent)
      handleSnapshotChunk(chunk1)
      handleSnapshotChunk(chunk2)

      // Verify highest seq is tracked by checking buffering
      const liveEvent: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:03Z',
        sequence: 109,
        protocol_version: 2,
      }

      // Should NOT buffer (109 < 110)
      bufferEventDuringSnapshot('proj1', 'tick1', liveEvent)
      const buffered = drainBufferedEvents('proj1:tick1')
      expect(buffered.length).toBe(0)
    })

    it('handleSnapshotChunk handles all entity types', () => {
      const entities: SnapshotEntityType[] = [
        'workflow_state',
        'agent_sessions',
        'findings',
        'ticket_detail',
        'chain_status',
      ]

      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)

      entities.forEach((entity, i) => {
        const chunk: WSEventV2 = {
          type: 'snapshot.chunk',
          project_id: 'proj1',
          ticket_id: 'tick1',
          timestamp: '2026-02-14T00:00:00Z',
          sequence: 101 + i,
          protocol_version: 2,
          entity,
          data: { test: entity },
        }

        expect(() => handleSnapshotChunk(chunk)).not.toThrow()
      })

      const endEvent: WSEventV2 = {
        type: 'snapshot.end',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:10Z',
        sequence: 110,
        protocol_version: 2,
      }

      vi.spyOn(queryClient, 'invalidateQueries')
      handleSnapshotEnd(endEvent, queryClient)

      // All entity types should trigger cache invalidation
      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })
  })

  describe('event buffering during snapshot', () => {
    it('bufferEventDuringSnapshot returns false when not receiving', () => {
      const event: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 200,
        protocol_version: 2,
      }

      const buffered = bufferEventDuringSnapshot('proj1', 'tick1', event)
      expect(buffered).toBe(false)
    })

    it('bufferEventDuringSnapshot buffers live events during snapshot', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)

      const liveEvent: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 150,
        protocol_version: 2,
      }

      const buffered = bufferEventDuringSnapshot('proj1', 'tick1', liveEvent)
      expect(buffered).toBe(true)
    })

    it('bufferEventDuringSnapshot only buffers events with seq > snapshot seq', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)

      const oldEvent: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 95,
        protocol_version: 2,
      }

      bufferEventDuringSnapshot('proj1', 'tick1', oldEvent)
      const buffered = drainBufferedEvents('proj1:tick1')
      expect(buffered.length).toBe(0)
    })

    it('drainBufferedEvents returns and clears buffered events', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)

      const event1: WSEventV2 = {
        type: 'agent.started',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 150,
        protocol_version: 2,
      }

      const event2: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:02Z',
        sequence: 151,
        protocol_version: 2,
      }

      bufferEventDuringSnapshot('proj1', 'tick1', event1)
      bufferEventDuringSnapshot('proj1', 'tick1', event2)

      const buffered = drainBufferedEvents('proj1:tick1')
      expect(buffered.length).toBe(2)
      expect(buffered[0].sequence).toBe(150)
      expect(buffered[1].sequence).toBe(151)

      // Second drain should be empty
      const buffered2 = drainBufferedEvents('proj1:tick1')
      expect(buffered2.length).toBe(0)
    })
  })

  describe('handleSnapshotEnd - buffered event replay', () => {
    it('returns buffered events in insertion order', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)

      const chunk: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 105,
        protocol_version: 2,
        entity: 'workflow_state',
        data: {},
      }

      handleSnapshotChunk(chunk)

      // Buffer events in order
      const event1: WSEventV2 = {
        type: 'agent.started',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:02Z',
        sequence: 106,
        protocol_version: 2,
      }

      const event2: WSEventV2 = {
        type: 'workflow.updated',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:03Z',
        sequence: 107,
        protocol_version: 2,
      }

      bufferEventDuringSnapshot('proj1', 'tick1', event1)
      bufferEventDuringSnapshot('proj1', 'tick1', event2)

      const endEvent: WSEventV2 = {
        type: 'snapshot.end',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:04Z',
        sequence: 108,
        protocol_version: 2,
      }

      const buffered = handleSnapshotEnd(endEvent, queryClient)
      expect(buffered.length).toBe(2)
      // Events returned in insertion order
      expect(buffered[0].sequence).toBe(106)
      expect(buffered[1].sequence).toBe(107)
    })

    it('applies all chunks to cache', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)

      const chunk1: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 101,
        protocol_version: 2,
        entity: 'workflow_state',
        data: { state: 'running' },
      }

      const chunk2: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:02Z',
        sequence: 102,
        protocol_version: 2,
        entity: 'agent_sessions',
        data: { sessions: [] },
      }

      handleSnapshotChunk(chunk1)
      handleSnapshotChunk(chunk2)

      const endEvent: WSEventV2 = {
        type: 'snapshot.end',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:03Z',
        sequence: 103,
        protocol_version: 2,
      }

      vi.spyOn(queryClient, 'invalidateQueries')
      handleSnapshotEnd(endEvent, queryClient)

      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })

    it('handles snapshot with no chunks', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)

      const endEvent: WSEventV2 = {
        type: 'snapshot.end',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 101,
        protocol_version: 2,
      }

      const buffered = handleSnapshotEnd(endEvent, queryClient)
      expect(buffered.length).toBe(0)
      expect(isReceivingSnapshot('proj1', 'tick1')).toBe(false)
    })

    it('handles snapshot.end when not receiving', () => {
      const endEvent: WSEventV2 = {
        type: 'snapshot.end',
        project_id: 'proj1',
        ticket_id: 'tick1',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 101,
        protocol_version: 2,
      }

      const buffered = handleSnapshotEnd(endEvent, queryClient)
      expect(buffered.length).toBe(0)
    })
  })

  describe('project-scoped snapshots', () => {
    it('handles project-scoped snapshot', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: '', // Empty ticket_id = project scope
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)
      expect(isReceivingSnapshot('proj1', '')).toBe(true)

      const chunk: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: '',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 101,
        protocol_version: 2,
        entity: 'workflow_state',
        data: {},
      }

      handleSnapshotChunk(chunk)

      const endEvent: WSEventV2 = {
        type: 'snapshot.end',
        project_id: 'proj1',
        ticket_id: '',
        timestamp: '2026-02-14T00:00:02Z',
        sequence: 102,
        protocol_version: 2,
      }

      vi.spyOn(queryClient, 'invalidateQueries')
      handleSnapshotEnd(endEvent, queryClient)

      expect(queryClient.invalidateQueries).toHaveBeenCalled()
      expect(isReceivingSnapshot('proj1', '')).toBe(false)
    })
  })

  describe('chain_status entity handling', () => {
    it('applies chain_status chunk with chain_id', () => {
      const beginEvent: WSEventV2 = {
        type: 'snapshot.begin',
        project_id: 'proj1',
        ticket_id: '',
        timestamp: '2026-02-14T00:00:00Z',
        sequence: 100,
        protocol_version: 2,
      }

      handleSnapshotBegin(beginEvent)

      const chunk: WSEventV2 = {
        type: 'snapshot.chunk',
        project_id: 'proj1',
        ticket_id: '',
        timestamp: '2026-02-14T00:00:01Z',
        sequence: 101,
        protocol_version: 2,
        entity: 'chain_status',
        data: { chain_id: 'chain-123', status: 'running' },
      }

      handleSnapshotChunk(chunk)

      const endEvent: WSEventV2 = {
        type: 'snapshot.end',
        project_id: 'proj1',
        ticket_id: '',
        timestamp: '2026-02-14T00:00:02Z',
        sequence: 102,
        protocol_version: 2,
      }

      vi.spyOn(queryClient, 'invalidateQueries')
      handleSnapshotEnd(endEvent, queryClient)

      expect(queryClient.invalidateQueries).toHaveBeenCalled()
    })
  })
})
