// vi.hoisted runs before static imports — set up localStorage so the
// connectionsStore persist middleware can write without throwing.
vi.hoisted(() => {
  const data: Record<string, string> = {}
  Object.defineProperty(globalThis, 'localStorage', {
    value: {
      getItem: (k: string) => data[k] ?? null,
      setItem: (k: string, v: string) => { data[k] = v },
      removeItem: (k: string) => { delete data[k] },
      clear: () => { for (const k of Object.keys(data)) delete data[k] },
    },
    writable: true,
    configurable: true,
  })
})

import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { useInteractiveSessionsStore } from './interactiveSessionsStore'
import { useConnectionsStore } from './connectionsStore'
import type { InteractiveSession } from './interactiveSessionsStore'
import type { Connection } from './connectionsStore'

// Importing interactiveSessionsStore activates the module-level
// useConnectionsStore.subscribe() that clears sessions on activeId change.

const makeSession = (overrides: Partial<InteractiveSession> = {}): InteractiveSession => ({
  sessionId: 'sess-1',
  agentType: 'setup-analyzer',
  scope: { type: 'ticket', ticketId: 'TICKET-1' },
  workflow: 'feature',
  startedAt: 1000,
  ...overrides,
})

const LOCAL: Connection = { id: 'local', name: 'Local', baseURL: '', isLocal: true }

const REMOTE: Connection = {
  id: 'remote-switch',
  name: 'Remote',
  baseURL: 'https://remote.example',
  isLocal: false,
  token: 'tok',
}

describe('interactiveSessionsStore — connection switch', () => {
  beforeEach(() => {
    useInteractiveSessionsStore.setState({ sessions: [], activeId: '', minimized: false })
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
  })

  afterEach(() => {
    useInteractiveSessionsStore.setState({ sessions: [], activeId: '', minimized: false })
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
  })

  it('clears sessions when active connection switches to remote', () => {
    useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'a' }))
    useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'b' }))
    expect(useInteractiveSessionsStore.getState().sessions).toHaveLength(2)

    useConnectionsStore.getState().add(REMOTE)
    useConnectionsStore.getState().setActive('remote-switch')

    const s = useInteractiveSessionsStore.getState()
    expect(s.sessions).toHaveLength(0)
    expect(s.activeId).toBe('')
    expect(s.minimized).toBe(false)
  })

  it('clears sessions when switching back to local', () => {
    useConnectionsStore.getState().add(REMOTE)
    useConnectionsStore.getState().setActive('remote-switch')

    useInteractiveSessionsStore.getState().add(makeSession({ sessionId: 'remote-sess' }))
    expect(useInteractiveSessionsStore.getState().sessions).toHaveLength(1)

    useConnectionsStore.getState().setActive('local')

    expect(useInteractiveSessionsStore.getState().sessions).toHaveLength(0)
  })

  it('clears minimized state alongside sessions on switch', () => {
    useInteractiveSessionsStore.getState().add(makeSession())
    useInteractiveSessionsStore.setState({ minimized: true })

    useConnectionsStore.getState().add(REMOTE)
    useConnectionsStore.getState().setActive('remote-switch')

    expect(useInteractiveSessionsStore.getState().minimized).toBe(false)
  })

  it('does not clear sessions on non-activeId store changes', () => {
    useInteractiveSessionsStore.getState().add(makeSession())
    expect(useInteractiveSessionsStore.getState().sessions).toHaveLength(1)

    // setActiveProject mutates the list but does NOT change activeId
    useConnectionsStore.getState().add(REMOTE)
    useConnectionsStore.getState().setActiveProject('remote-switch', 'proj-x')

    expect(useInteractiveSessionsStore.getState().sessions).toHaveLength(1)
  })
})
