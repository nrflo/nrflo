import { beforeEach, describe, it, expect } from 'vitest'
import { useInteractiveSessionsStore } from './interactiveSessionsStore'
import type { InteractiveSession } from './interactiveSessionsStore'

const makeSession = (overrides: Partial<InteractiveSession> = {}): InteractiveSession => ({
  sessionId: 'sess-1',
  agentType: 'setup-analyzer',
  scope: { type: 'ticket', ticketId: 'TICKET-1' },
  workflow: 'feature',
  startedAt: 1000,
  ...overrides,
})

describe('interactiveSessionsStore', () => {
  beforeEach(() => {
    useInteractiveSessionsStore.setState({ sessions: [], activeId: '', minimized: false })
  })

  describe('add', () => {
    it('appends session and sets activeId', () => {
      useInteractiveSessionsStore.getState().add(makeSession())
      const s = useInteractiveSessionsStore.getState()
      expect(s.sessions).toHaveLength(1)
      expect(s.activeId).toBe('sess-1')
    })

    it('sets minimized=false when adding', () => {
      useInteractiveSessionsStore.setState({ minimized: true })
      useInteractiveSessionsStore.getState().add(makeSession())
      expect(useInteractiveSessionsStore.getState().minimized).toBe(false)
    })

    it('does not duplicate session with same sessionId', () => {
      const session = makeSession()
      useInteractiveSessionsStore.getState().add(session)
      useInteractiveSessionsStore.getState().add(session)
      expect(useInteractiveSessionsStore.getState().sessions).toHaveLength(1)
    })

    it('updates activeId to newly added session', () => {
      useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'a' }))
      useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'b' }))
      expect(useInteractiveSessionsStore.getState().activeId).toBe('b')
    })
  })

  describe('remove', () => {
    it('removes session and clears activeId when list empties', () => {
      useInteractiveSessionsStore.getState().add(makeSession())
      useInteractiveSessionsStore.getState().remove('sess-1')
      const s = useInteractiveSessionsStore.getState()
      expect(s.sessions).toHaveLength(0)
      expect(s.activeId).toBe('')
      expect(s.minimized).toBe(false)
    })

    it('switches activeId to last remaining session when active is removed', () => {
      useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'a' }))
      useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'b' }))
      useInteractiveSessionsStore.getState().remove('b')
      expect(useInteractiveSessionsStore.getState().activeId).toBe('a')
    })

    it('preserves activeId when a non-active session is removed', () => {
      useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'a' }))
      useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'b' }))
      useInteractiveSessionsStore.getState().setActive('a')
      useInteractiveSessionsStore.getState().remove('b')
      expect(useInteractiveSessionsStore.getState().activeId).toBe('a')
    })

    it('preserves minimized state when sessions remain', () => {
      useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'a' }))
      useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'b' }))
      useInteractiveSessionsStore.setState({ minimized: true })
      useInteractiveSessionsStore.getState().remove('b')
      expect(useInteractiveSessionsStore.getState().minimized).toBe(true)
    })
  })

  describe('setActive', () => {
    it('updates activeId', () => {
      useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'x' }))
      useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'y' }))
      useInteractiveSessionsStore.getState().setActive('x')
      expect(useInteractiveSessionsStore.getState().activeId).toBe('x')
    })
  })

  describe('toggleMinimized', () => {
    it('flips minimized from false to true', () => {
      useInteractiveSessionsStore.getState().toggleMinimized()
      expect(useInteractiveSessionsStore.getState().minimized).toBe(true)
    })

    it('flips minimized from true to false', () => {
      useInteractiveSessionsStore.setState({ minimized: true })
      useInteractiveSessionsStore.getState().toggleMinimized()
      expect(useInteractiveSessionsStore.getState().minimized).toBe(false)
    })
  })
})
