import { useState } from 'react'
import { TableRow, TableCell } from '@/components/ui/Table'
import { Button } from '@/components/ui/Button'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { useConnectionsStore, type Connection } from '@/stores/connectionsStore'
import { testConnection } from '@/api/client'

interface ConnectionRowProps {
  conn: Connection
  isActive: boolean
}

export function ConnectionRow({ conn, isActive }: ConnectionRowProps) {
  const { remove, setActive } = useConnectionsStore()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [testResult, setTestResult] = useState<{ ok: boolean; message?: string } | null>(null)
  const [testing, setTesting] = useState(false)

  const handleTest = async () => {
    setTesting(true)
    setTestResult(null)
    const result = await testConnection(conn)
    setTestResult(result)
    setTesting(false)
  }

  const handleRemove = () => {
    if (isActive) setActive('local')
    remove(conn.id)
  }

  const statusLabel = conn.authFailed ? 'Authentication failed' : isActive ? 'Active' : ''

  if (conn.isLocal) {
    return (
      <TableRow>
        <TableCell className="font-medium">{conn.name}</TableCell>
        <TableCell className="text-muted-foreground">—</TableCell>
        <TableCell>
          <span className="text-xs text-muted-foreground">Active</span>
        </TableCell>
        <TableCell />
        <TableCell>
          <Button variant="outline" size="sm" disabled>Remove</Button>
        </TableCell>
      </TableRow>
    )
  }

  return (
    <>
      <TableRow>
        <TableCell className="font-medium">{conn.name}</TableCell>
        <TableCell className="text-muted-foreground text-xs font-mono">{conn.baseURL}</TableCell>
        <TableCell>
          {statusLabel && (
            <span className={conn.authFailed ? 'text-xs text-destructive' : 'text-xs text-muted-foreground'}>
              {statusLabel}
            </span>
          )}
        </TableCell>
        <TableCell>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={handleTest} disabled={testing}>
              {testing ? 'Testing…' : 'Test'}
            </Button>
            {testResult !== null && (
              <span className={testResult.ok ? 'text-xs text-green-600' : 'text-xs text-destructive'}>
                {testResult.ok ? 'OK' : testResult.message ?? 'Failed'}
              </span>
            )}
          </div>
        </TableCell>
        <TableCell>
          <Button variant="destructive" size="sm" onClick={() => setConfirmOpen(true)}>Remove</Button>
        </TableCell>
      </TableRow>

      <ConfirmDialog
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        onConfirm={handleRemove}
        title="Remove connection"
        message={`Remove "${conn.name}"? This cannot be undone.`}
        confirmLabel="Remove"
        variant="destructive"
      />
    </>
  )
}
