import { useState } from 'react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { useConnectionsStore } from '@/stores/connectionsStore'
import { testConnection } from '@/api/client'

interface ConnectionAddDialogProps {
  open: boolean
  onClose: () => void
}

function isAbsoluteHttpUrl(url: string): boolean {
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

export function ConnectionAddDialog({ open, onClose }: ConnectionAddDialogProps) {
  const add = useConnectionsStore((s) => s.add)
  const [name, setName] = useState('')
  const [baseURL, setBaseURL] = useState('')
  const [token, setToken] = useState('')
  const [testResult, setTestResult] = useState<{ ok: boolean; message?: string } | null>(null)
  const [testing, setTesting] = useState(false)

  const urlValid = isAbsoluteHttpUrl(baseURL.trim())
  const canSave = name.trim() && urlValid && token.trim()

  const handleTest = async () => {
    setTesting(true)
    setTestResult(null)
    const result = await testConnection({
      id: '__preview__',
      name: name.trim() || 'preview',
      baseURL: baseURL.trim(),
      isLocal: false,
      token: token.trim(),
    })
    setTestResult(result)
    setTesting(false)
  }

  const handleSubmit = () => {
    if (!canSave) return
    add({
      id: crypto.randomUUID(),
      name: name.trim(),
      baseURL: baseURL.trim(),
      isLocal: false,
      token: token.trim(),
    })
    handleClose()
  }

  const handleClose = () => {
    setName('')
    setBaseURL('')
    setToken('')
    setTestResult(null)
    onClose()
  }

  return (
    <Dialog open={open} onClose={handleClose} className="max-w-lg">
      <DialogHeader onClose={handleClose}>
        <h3 className="text-lg font-semibold">Add Connection</h3>
      </DialogHeader>
      <DialogBody className="space-y-4">
        <div className="space-y-1">
          <label className="text-sm font-medium">Name</label>
          <Input placeholder="My nrflo server" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Base URL</label>
          <Input
            placeholder="https://nrflo.example.com"
            value={baseURL}
            onChange={(e) => { setBaseURL(e.target.value); setTestResult(null) }}
          />
          {baseURL && !urlValid && (
            <p className="text-xs text-destructive">Must be an absolute http(s) URL.</p>
          )}
          <p className="text-xs text-muted-foreground">
            Ensure the server allows cross-origin requests (CORS) from this browser origin.
          </p>
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Service Token</label>
          <Input
            type="password"
            placeholder="nrf_…"
            value={token}
            onChange={(e) => { setToken(e.target.value); setTestResult(null) }}
          />
        </div>
        <div className="flex items-center gap-3">
          <Button variant="outline" size="sm" onClick={handleTest} disabled={testing || !urlValid || !token.trim()}>
            {testing ? 'Testing…' : 'Test connection'}
          </Button>
          {testResult !== null && (
            <span className={testResult.ok ? 'text-sm text-green-600' : 'text-sm text-destructive'}>
              {testResult.ok ? 'Connection successful' : testResult.message ?? 'Failed'}
            </span>
          )}
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" size="sm" onClick={handleClose}>Cancel</Button>
        <Button size="sm" onClick={handleSubmit} disabled={!canSave}>Save</Button>
      </DialogFooter>
    </Dialog>
  )
}
