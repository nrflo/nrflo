import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { deliverDesktopNotification, enableDesktopNotifications } from './desktopNotifications'
import { getInboxNotification } from '@/api/notificationInbox'

vi.mock('@/api/notificationInbox', () => ({ getInboxNotification: vi.fn() }))
vi.mock('@/stores/connectionsStore', () => ({ useConnectionsStore: { getState: () => ({ activeId: 'local' }) } }))

const showNotification = vi.fn().mockResolvedValue(undefined)
const register = vi.fn().mockResolvedValue({})
const requestPermission = vi.fn().mockResolvedValue('granted')
const event = { type: 'global.notifications' as const, project_id: '', ticket_id: '', timestamp: '', data: { notification_id: 'note-1' } }

beforeEach(() => {
  vi.clearAllMocks()
  const storage = new Map<string, string>()
  vi.stubGlobal('localStorage', { getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => storage.set(key, value) })
  vi.stubGlobal('window', { isSecureContext: true })
  vi.stubGlobal('Notification', { permission: 'granted', requestPermission })
  vi.stubGlobal('navigator', { serviceWorker: { register, ready: Promise.resolve({ getNotifications: async () => [], showNotification }) } })
  vi.mocked(getInboxNotification).mockResolvedValue({ id: 'note-1', project_id: 'p', ticket_id: '', workflow_id: 'wf', event_type: 'orchestration.completed', payload: { project_name: 'Project', workflow_final_result: 'Done' }, created_at: '', read_at: null })
})
afterEach(() => vi.unstubAllGlobals())

describe('desktop notifications', () => {
  it('requests permission only on explicit enable', async () => {
    await deliverDesktopNotification(event)
    expect(requestPermission).not.toHaveBeenCalled()
    expect(showNotification).not.toHaveBeenCalled()
    await enableDesktopNotifications()
    expect(requestPermission).toHaveBeenCalledOnce()
    expect(register).toHaveBeenCalledWith('/notification-worker.js')
  })

  it('delivers new arrivals once with the exact item and connection URL', async () => {
    await enableDesktopNotifications()
    await deliverDesktopNotification(event)
    await deliverDesktopNotification(event)
    expect(showNotification).toHaveBeenCalledOnce()
    expect(showNotification).toHaveBeenCalledWith('nrflo · Project', expect.objectContaining({ data: { url: '/notifications?scope=all&id=note-1&connection=local' } }))
  })

  it('does not alert for read-state changes or already-read items', async () => {
    await enableDesktopNotifications()
    await deliverDesktopNotification({ ...event, data: {} })
    vi.mocked(getInboxNotification).mockResolvedValue({ ...(await getInboxNotification(true, 'note-1')), read_at: 'now' })
    await deliverDesktopNotification(event)
    expect(showNotification).not.toHaveBeenCalled()
  })
})
