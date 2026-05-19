import { create } from 'zustand'
import { useConnectionsStore } from './connectionsStore'

export interface InteractiveSession {
  sessionId: string
  agentType: string
  scope: { type: 'ticket'; ticketId: string } | { type: 'project'; projectId: string }
  workflow: string
  instanceId?: string
  startedAt: number
}

interface InteractiveSessionsState {
  sessions: InteractiveSession[]
  activeId: string
  minimized: boolean
  add: (session: InteractiveSession) => void
  remove: (sessionId: string) => void
  setActive: (sessionId: string) => void
  toggleMinimized: () => void
}

export const useInteractiveSessionsStore = create<InteractiveSessionsState>()((set) => ({
  sessions: [],
  activeId: '',
  minimized: false,
  add: (session) =>
    set((state) => ({
      sessions: state.sessions.some((s) => s.sessionId === session.sessionId)
        ? state.sessions
        : [...state.sessions, session],
      activeId: session.sessionId,
      minimized: false,
    })),
  remove: (sessionId) =>
    set((state) => {
      const next = state.sessions.filter((s) => s.sessionId !== sessionId)
      const nextActiveId = state.activeId === sessionId
        ? (next.length > 0 ? next[next.length - 1].sessionId : '')
        : state.activeId
      return {
        sessions: next,
        activeId: nextActiveId,
        minimized: next.length === 0 ? false : state.minimized,
      }
    }),
  setActive: (sessionId) => set({ activeId: sessionId }),
  toggleMinimized: () => set((state) => ({ minimized: !state.minimized })),
}))

// Sessions belong to the previously-active server; clear on connection switch.
// zustand 5: subscribe takes a single (newState, prevState) listener — the
// 2-arg selector form requires subscribeWithSelector middleware.
useConnectionsStore.subscribe((newState, prevState) => {
  if (newState.activeId !== prevState.activeId) {
    useInteractiveSessionsStore.setState({ sessions: [], activeId: '', minimized: false })
  }
})
