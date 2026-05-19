import { useNavigate } from 'react-router-dom'
import { useConnectionsStore } from '@/stores/connectionsStore'
import { Button } from '@/components/ui/Button'

export function AuthFailedBanner() {
  const navigate = useNavigate()
  const failedConn = useConnectionsStore((s) => s.list.find((c) => !c.isLocal && c.authFailed))

  if (!failedConn) return null

  return (
    <div className="bg-destructive/10 border-b border-destructive/20 px-4 py-2 text-sm text-destructive flex items-center gap-2">
      <span>
        Service token for <strong>{failedConn.name}</strong> was rejected.
      </span>
      <Button variant="link" size="sm" className="text-destructive p-0 h-auto" onClick={() => navigate('/settings/connections')}>
        Open Connections
      </Button>
      <span>to re-paste.</span>
    </div>
  )
}
