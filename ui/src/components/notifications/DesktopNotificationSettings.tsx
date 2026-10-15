import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { desktopNotificationsEnabled, desktopNotificationsSupported, disableDesktopNotifications, enableDesktopNotifications } from '@/lib/desktopNotifications'

export function DesktopNotificationSettings() {
  const [enabled, setEnabled] = useState(desktopNotificationsEnabled)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const supported = desktopNotificationsSupported()
  const enable = async () => {
    setPending(true)
    setError('')
    try {
      await enableDesktopNotifications()
      setEnabled(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not enable desktop notifications.')
    } finally {
      setPending(false)
    }
  }
  return <div className="rounded-lg border border-border p-4 space-y-3">
    <h3 className="font-semibold">OS notifications</h3>
    <p className="text-sm text-muted-foreground">Show new notifications in your operating system’s notification center. Clicking an alert opens the exact inbox item. Keep an nrflo tab open to receive alerts; this preference applies to this browser.</p>
    {!supported && <p className="text-sm text-muted-foreground">Use a supported browser on HTTPS or localhost to enable OS notifications.</p>}
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    {enabled ? <Button variant="outline" onClick={() => { disableDesktopNotifications(); setEnabled(false) }}>Disable OS notifications</Button>
      : <Button variant="outline" disabled={!supported || pending} onClick={enable}>{pending ? 'Enabling…' : 'Enable OS notifications'}</Button>}
  </div>
}
