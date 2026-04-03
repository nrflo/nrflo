import { useState, useRef, useEffect, useCallback } from 'react'
import { Zap, Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { testCLIModel } from '@/api/cliModels'

type TestStatus = 'idle' | 'testing' | 'success' | 'error'

interface CLIModelCheckButtonProps {
  modelId: string
  disabled?: boolean
}

export function CLIModelCheckButton({ modelId, disabled }: CLIModelCheckButtonProps) {
  const [status, setStatus] = useState<TestStatus>('idle')
  const [error, setError] = useState('')
  const [durationMs, setDurationMs] = useState(0)
  const [showErrorDialog, setShowErrorDialog] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
      abortRef.current?.abort()
    }
  }, [])

  const handleTest = useCallback(async () => {
    setStatus('testing')
    setError('')
    setShowErrorDialog(false)
    const controller = new AbortController()
    abortRef.current = controller
    const timeoutId = setTimeout(() => controller.abort(), 45_000)
    try {
      const result = await testCLIModel(modelId, controller.signal)
      if (result.success) {
        setStatus('success')
        setDurationMs(result.duration_ms)
        timerRef.current = setTimeout(() => {
          setStatus('idle')
          timerRef.current = null
        }, 3000)
      } else {
        setStatus('error')
        setError(result.error || 'Unknown error')
      }
    } catch (err) {
      setStatus('error')
      if (err instanceof DOMException && err.name === 'AbortError') {
        setError('Timeout — server did not respond')
      } else {
        setError((err as Error).message)
      }
    } finally {
      clearTimeout(timeoutId)
    }
  }, [modelId])

  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        title="Check model"
        disabled={disabled || status === 'testing'}
        onClick={handleTest}
      >
        {status === 'testing' ? (
          <Spinner className="h-4 w-4" />
        ) : status === 'success' ? (
          <Check className="h-4 w-4 text-green-500" />
        ) : (
          <Zap className="h-4 w-4" />
        )}
      </Button>
      {status === 'success' && (
        <span className="text-xs text-green-600 dark:text-green-400">{durationMs}ms</span>
      )}
      {status === 'error' && (
        <button
          type="button"
          aria-label="Show error details"
          className="p-1 rounded hover:bg-muted transition-colors"
          onClick={() => setShowErrorDialog(true)}
        >
          <Zap className="h-4 w-4 text-red-500" />
        </button>
      )}
      <Dialog open={showErrorDialog} onClose={() => setShowErrorDialog(false)}>
        <DialogHeader onClose={() => setShowErrorDialog(false)}>
          Model Check Error — {modelId}
        </DialogHeader>
        <DialogBody>
          <pre className="whitespace-pre-wrap break-words text-sm">{error}</pre>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => setShowErrorDialog(false)}>Close</Button>
        </DialogFooter>
      </Dialog>
    </>
  )
}
