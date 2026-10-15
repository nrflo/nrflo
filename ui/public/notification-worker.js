self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()))

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const target = new URL(event.notification.data?.url ?? '/notifications?scope=all', self.location.origin)
  if (target.origin !== self.location.origin || target.pathname !== '/notifications') return
  event.waitUntil((async () => {
    const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
    const existing = windows.find((client) => new URL(client.url).origin === target.origin)
    if (existing) {
      const navigated = await existing.navigate(target.href)
      if (navigated) return navigated.focus()
    }
    return self.clients.openWindow(target.href)
  })())
})
