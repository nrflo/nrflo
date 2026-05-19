import { create } from 'zustand'
import { persist, createJSONStorage } from 'zustand/middleware'

export interface Connection {
  id: string
  name: string
  baseURL: string
  isLocal: boolean
  token?: string
  activeProject?: string
  authFailed?: boolean
}

const LOCAL_CONNECTION: Connection = {
  id: 'local',
  name: 'Local',
  baseURL: '',
  isLocal: true,
}

interface ConnectionsState {
  list: Connection[]
  activeId: string
  add: (conn: Connection) => void
  remove: (id: string) => void
  setActive: (id: string) => void
  setActiveProject: (connectionId: string, projectId: string) => void
  markAuthFailed: (id: string) => void
  update: (id: string, updates: Partial<Omit<Connection, 'id'>>) => void
  active: () => Connection
}

export const useConnectionsStore = create<ConnectionsState>()(
  persist(
    (set, get) => ({
      list: [LOCAL_CONNECTION],
      activeId: 'local',
      add: (conn) => set((s) => ({ list: [...s.list, conn] })),
      remove: (id) => {
        if (id === 'local') return
        set((s) => ({ list: s.list.filter((c) => c.id !== id) }))
      },
      setActive: (id) => set({ activeId: id }),
      setActiveProject: (connectionId, projectId) =>
        set((s) => ({
          list: s.list.map((c) =>
            c.id === connectionId ? { ...c, activeProject: projectId } : c
          ),
        })),
      markAuthFailed: (id) =>
        set((s) => ({
          list: s.list.map((c) =>
            c.id === id ? { ...c, authFailed: true } : c
          ),
        })),
      update: (id, updates) =>
        set((s) => ({
          list: s.list.map((c) =>
            c.id === id ? { ...c, ...updates } : c
          ),
        })),
      active: () => {
        const { list, activeId } = get()
        return list.find((c) => c.id === activeId) ?? LOCAL_CONNECTION
      },
    }),
    {
      name: 'nrf_connections',
      storage: createJSONStorage(() => localStorage),
      partialize: (state) => ({ list: state.list, activeId: state.activeId }),
      merge: (persistedState, currentState) => {
        const persisted = persistedState as { list?: Connection[]; activeId?: string }
        const list = persisted.list ?? []
        const hasLocal = list.some((c) => c.id === 'local')
        return {
          ...currentState,
          ...persisted,
          list: hasLocal ? list : [LOCAL_CONNECTION, ...list],
          activeId: persisted.activeId ?? 'local',
        }
      },
    }
  )
)

export function useActiveConnection(): Connection {
  return useConnectionsStore((s) => s.list.find((c) => c.id === s.activeId) ?? LOCAL_CONNECTION)
}
