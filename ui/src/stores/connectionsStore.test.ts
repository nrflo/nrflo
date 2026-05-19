import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { Connection } from './connectionsStore'

// localStorage is not available in threads-pool jsdom — mock it directly
const localStorageData: Record<string, string> = {}
const mockLocalStorage = {
  getItem: (key: string) => localStorageData[key] ?? null,
  setItem: (key: string, value: string) => { localStorageData[key] = value },
  removeItem: (key: string) => { delete localStorageData[key] },
}
Object.defineProperty(global, 'localStorage', {
  writable: true,
  configurable: true,
  value: mockLocalStorage,
})

const KEY = 'nrf_connections'

function storedState(): { list?: Connection[]; activeId?: string } | null {
  const raw = localStorageData[KEY]
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw)
    return parsed.state ?? null
  } catch { return null }
}

function makeRemote(overrides: Partial<Connection> = {}): Connection {
  return {
    id: 'remote-1',
    name: 'Production',
    baseURL: 'https://prod.example.com',
    isLocal: false,
    token: 'tok-abc',
    ...overrides,
  }
}

async function freshStore(preloaded?: object) {
  vi.resetModules()
  delete localStorageData[KEY]
  if (preloaded) {
    localStorageData[KEY] = JSON.stringify({ state: preloaded, version: 0 })
  }
  const { useConnectionsStore } = await import('./connectionsStore')
  return useConnectionsStore
}

describe('connectionsStore — initial seed', () => {
  it('creates Local connection when localStorage is empty', async () => {
    const store = await freshStore()
    const { list } = store.getState()
    expect(list).toHaveLength(1)
    expect(list[0].id).toBe('local')
    expect(list[0].isLocal).toBe(true)
    expect(list[0].baseURL).toBe('')
  })

  it('activeId defaults to local', async () => {
    const store = await freshStore()
    expect(store.getState().activeId).toBe('local')
  })

  it('injects Local when persisted list lacks it', async () => {
    const remote = makeRemote()
    const store = await freshStore({ list: [remote], activeId: 'remote-1' })
    const { list } = store.getState()
    expect(list.some((c) => c.id === 'local')).toBe(true)
    expect(list.some((c) => c.id === 'remote-1')).toBe(true)
  })

  it('does not duplicate Local when persisted list already has it', async () => {
    const local = { id: 'local', name: 'Local', baseURL: '', isLocal: true }
    const store = await freshStore({ list: [local], activeId: 'local' })
    const localEntries = store.getState().list.filter((c) => c.id === 'local')
    expect(localEntries).toHaveLength(1)
  })

  it('restores activeId from persisted state', async () => {
    const local = { id: 'local', name: 'Local', baseURL: '', isLocal: true }
    const remote = makeRemote()
    const store = await freshStore({ list: [local, remote], activeId: 'remote-1' })
    expect(store.getState().activeId).toBe('remote-1')
  })
})

describe('connectionsStore — add()', () => {
  beforeEach(() => { vi.resetModules(); delete localStorageData[KEY] })

  it('appends connection to list', async () => {
    const store = await freshStore()
    const remote = makeRemote()
    store.getState().add(remote)
    expect(store.getState().list).toHaveLength(2)
    expect(store.getState().list[1]).toMatchObject({ id: 'remote-1', name: 'Production' })
  })

  it('persists to nrf_connections key in localStorage', async () => {
    const store = await freshStore()
    store.getState().add(makeRemote())
    const persisted = storedState()
    expect(persisted?.list?.some((c: Connection) => c.id === 'remote-1')).toBe(true)
  })
})

describe('connectionsStore — remove()', () => {
  beforeEach(() => { vi.resetModules(); delete localStorageData[KEY] })

  it('is a no-op for the local connection', async () => {
    const store = await freshStore()
    store.getState().remove('local')
    expect(store.getState().list.some((c) => c.id === 'local')).toBe(true)
  })

  it('removes remote connection by id', async () => {
    const store = await freshStore()
    store.getState().add(makeRemote())
    store.getState().remove('remote-1')
    expect(store.getState().list.some((c) => c.id === 'remote-1')).toBe(false)
  })

  it('persists removal to nrf_connections', async () => {
    const store = await freshStore()
    store.getState().add(makeRemote())
    store.getState().remove('remote-1')
    const persisted = storedState()
    expect(persisted?.list?.some((c: Connection) => c.id === 'remote-1')).toBe(false)
  })
})

describe('connectionsStore — setActive()', () => {
  beforeEach(() => { vi.resetModules(); delete localStorageData[KEY] })

  it('switches activeId', async () => {
    const store = await freshStore()
    store.getState().add(makeRemote())
    store.getState().setActive('remote-1')
    expect(store.getState().activeId).toBe('remote-1')
  })

  it('persists new activeId', async () => {
    const store = await freshStore()
    store.getState().add(makeRemote())
    store.getState().setActive('remote-1')
    expect(storedState()?.activeId).toBe('remote-1')
  })
})

describe('connectionsStore — setActiveProject()', () => {
  beforeEach(() => { vi.resetModules(); delete localStorageData[KEY] })

  it('sets activeProject on targeted connection', async () => {
    const store = await freshStore()
    store.getState().add(makeRemote())
    store.getState().setActiveProject('remote-1', 'proj-x')
    const conn = store.getState().list.find((c) => c.id === 'remote-1')
    expect(conn?.activeProject).toBe('proj-x')
  })

  it('does not mutate other connections', async () => {
    const store = await freshStore()
    store.getState().add(makeRemote())
    store.getState().setActiveProject('remote-1', 'proj-x')
    const local = store.getState().list.find((c) => c.id === 'local')
    expect(local?.activeProject).toBeUndefined()
  })

  it('persists activeProject', async () => {
    const store = await freshStore()
    store.getState().add(makeRemote())
    store.getState().setActiveProject('remote-1', 'proj-x')
    const persisted = storedState()
    const conn = persisted?.list?.find((c: Connection) => c.id === 'remote-1')
    expect((conn as Connection)?.activeProject).toBe('proj-x')
  })
})

describe('connectionsStore — markAuthFailed()', () => {
  beforeEach(() => { vi.resetModules(); delete localStorageData[KEY] })

  it('sets authFailed=true on the targeted connection', async () => {
    const store = await freshStore()
    store.getState().add(makeRemote())
    store.getState().markAuthFailed('remote-1')
    const conn = store.getState().list.find((c) => c.id === 'remote-1')
    expect(conn?.authFailed).toBe(true)
  })

  it('does not affect other connections', async () => {
    const store = await freshStore()
    store.getState().add(makeRemote())
    store.getState().markAuthFailed('remote-1')
    const local = store.getState().list.find((c) => c.id === 'local')
    expect(local?.authFailed).toBeUndefined()
  })
})

describe('connectionsStore — active()', () => {
  beforeEach(() => { vi.resetModules(); delete localStorageData[KEY] })

  it('returns local connection by default', async () => {
    const store = await freshStore()
    expect(store.getState().active().id).toBe('local')
  })

  it('returns the connection matching activeId', async () => {
    const store = await freshStore()
    store.getState().add(makeRemote())
    store.getState().setActive('remote-1')
    expect(store.getState().active().id).toBe('remote-1')
  })

  it('falls back to LOCAL_CONNECTION when activeId has no match', async () => {
    const store = await freshStore()
    store.setState({ activeId: 'nonexistent' })
    const conn = store.getState().active()
    expect(conn.id).toBe('local')
    expect(conn.isLocal).toBe(true)
  })
})
