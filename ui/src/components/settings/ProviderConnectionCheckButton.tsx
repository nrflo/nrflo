import { useState, useRef, useEffect, useCallback } from 'react'
import { Zap, Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { checkCustomProviderConnection, type APIWire } from '@/api/customProviders'

type CheckStatus = 'idle' | 'checking' | 'success' | 'error'

interface ProviderConnectionCheckButtonProps {
  baseUrl: string
  apiKey: string
  apiWire: APIWire
  disabled?: boolean
}

export function ProviderConnectionCheckButton({ baseUrl, apiKey, apiWire, disabled }: ProviderConnectionCheckButtonProps) {
  const [status, setStatus] = useState<CheckStatus>('idle')
  const [error, setError] = useState('')
  const [models, setModels] = useState<string[]>([])
  const [showResultDialog, setShowResultDialog] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
      abortRef.current?.abort()
    }
  }, [])

  const handleCheck = useCallback(async () => {
    setStatus('checking')
    setError('')
    setModels([])
    setShowResultDialog(false)
    const controller = new AbortController()
    abortRef.current = controller
    const timeoutId = setTimeout(() => controller.abort(), 45_000)
    try {
      const result = await checkCustomProviderConnection(
        { base_url: baseUrl, api_key: apiKey, api_wire: apiWire },
        controller.signal,
      )
      if (result.ok) {
        setStatus('success')
        setModels(result.models)
        timerRef.current = setTimeout(() => {
          setStatus('idle')
          timerRef.current = null
        }, 3000)
      } else {
        setStatus('error')
        setError(result.error || 'Unknown error')
        setShowResultDialog(true)
      }
    } catch (err) {
      setStatus('error')
      if (err instanceof DOMException && err.name === 'AbortError') {
        setError('Timeout — server did not respond')
      } else {
        setError((err as Error).message)
      }
      setShowResultDialog(true)
    } finally {
      clearTimeout(timeoutId)
    }
  }, [baseUrl, apiKey, apiWire])

  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={disabled || !baseUrl.trim() || status === 'checking'}
        onClick={handleCheck}
      >
        {status === 'checking' ? (
          <Spinner className="mr-2 h-4 w-4" />
        ) : status === 'success' ? (
          <Check className="mr-2 h-4 w-4 text-green-500" />
        ) : (
          <Zap className="mr-2 h-4 w-4" />
        )}
        Check connection
      </Button>
      {status === 'success' && (
        <span className="text-xs text-green-600 dark:text-green-400">{models.length} model{models.length === 1 ? '' : 's'}</span>
      )}
      <Dialog open={showResultDialog} onClose={() => setShowResultDialog(false)}>
        <DialogHeader onClose={() => setShowResultDialog(false)}>Connection Check</DialogHeader>
        <DialogBody>
          <pre className="whitespace-pre-wrap break-words text-sm">{error}</pre>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => setShowResultDialog(false)}>Close</Button>
        </DialogFooter>
      </Dialog>
    </>
  )
}
