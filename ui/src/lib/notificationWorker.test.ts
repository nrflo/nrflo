import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { describe, expect, it, vi } from 'vitest'

const source = readFileSync(new URL('../../public/notification-worker.js', import.meta.url), 'utf8')

describe('OS notification click', () => {
  it('navigates an existing window to the exact notification and focuses it', async () => {
    const handlers: Record<string, (event: unknown) => void> = {}
    const focus = vi.fn()
    const navigate = vi.fn().mockResolvedValue({ focus })
    const self = {
      location: { origin: 'http://localhost:6587' },
      addEventListener: (name: string, handler: (event: unknown) => void) => { handlers[name] = handler },
      clients: { matchAll: async () => [{ url: 'http://localhost:6587/', navigate }], openWindow: vi.fn() },
    }
    runInNewContext(source, { self, URL })
    let completion: Promise<unknown> | undefined
    const close = vi.fn()
    handlers.notificationclick({ notification: { close, data: { url: '/notifications?scope=all&id=exact&connection=local' } }, waitUntil: (promise: Promise<unknown>) => { completion = promise } })
    await completion
    expect(close).toHaveBeenCalledOnce()
    expect(navigate).toHaveBeenCalledWith('http://localhost:6587/notifications?scope=all&id=exact&connection=local')
    expect(focus).toHaveBeenCalledOnce()
  })

  it('does not navigate to an external origin', () => {
    const handlers: Record<string, (event: unknown) => void> = {}
    runInNewContext(source, { self: { location: { origin: 'http://localhost:6587' }, addEventListener: (name: string, handler: (event: unknown) => void) => { handlers[name] = handler } }, URL })
    const waitUntil = vi.fn()
    handlers.notificationclick({ notification: { close: vi.fn(), data: { url: 'https://example.com/notifications' } }, waitUntil })
    expect(waitUntil).not.toHaveBeenCalled()
  })
})
