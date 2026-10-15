import { getInboxNotification } from '@/api/notificationInbox'
import { useConnectionsStore } from '@/stores/connectionsStore'
import type { WSEvent } from '@/hooks/useWebSocketTypes'

const preferenceKey = 'nrf_desktop_notifications'

export function desktopNotificationsSupported() {
  return typeof Notification !== 'undefined' && 'serviceWorker' in navigator && window.isSecureContext
}

export function desktopNotificationsEnabled() {
  return localStorage.getItem(preferenceKey) === 'true'
}

export function disableDesktopNotifications() {
  localStorage.setItem(preferenceKey, 'false')
}

export async function enableDesktopNotifications() {
  if (!desktopNotificationsSupported()) throw new Error('Desktop notifications require a supported browser on HTTPS or localhost.')
  const permission = await Notification.requestPermission()
  if (permission !== 'granted') throw new Error('Notifications are blocked. Allow notifications for nrflo in your browser and macOS settings, then try again.')
  await navigator.serviceWorker.register('/notification-worker.js')
  await navigator.serviceWorker.ready
  localStorage.setItem(preferenceKey, 'true')
}

export async function deliverDesktopNotification(event: WSEvent) {
  const id = event.data?.notification_id
  if (event.type !== 'global.notifications' || typeof id !== 'string' || !desktopNotificationsEnabled()) return
  if (!desktopNotificationsSupported() || Notification.permission !== 'granted') return
  const connection = useConnectionsStore.getState().activeId
  const item = await getInboxNotification(true, id)
  if (connection !== useConnectionsStore.getState().activeId || item.read_at) return
  await navigator.serviceWorker.register('/notification-worker.js')
  const registration = await navigator.serviceWorker.ready
  const tag = `nrflo:${connection}:${id}`
  const show = async () => {
    const seen: string[] = JSON.parse(localStorage.getItem('nrf_desktop_seen') || '[]')
    if (seen.includes(tag) || (await registration.getNotifications({ tag })).length) return
    const raw = item.payload.workflow_final_result || item.payload.reason || item.payload.error || item.event_type
    const body = (typeof raw === 'string' ? raw : JSON.stringify(raw)).slice(0, 240)
    const url = `/notifications?${new URLSearchParams({ scope: 'all', id, connection })}`
    await registration.showNotification(`nrflo · ${item.payload.project_name || item.project_id || 'General'}`, {
      body, tag, data: { url },
    })
    localStorage.setItem('nrf_desktop_seen', JSON.stringify([...seen.slice(-199), tag]))
  }
  if (navigator.locks) await navigator.locks.request('nrf-desktop-notifications', show)
  else await show()
}
