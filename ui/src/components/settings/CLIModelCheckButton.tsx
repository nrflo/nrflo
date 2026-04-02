import { useState, useRef, useEffect, useCallback } from 'react'
import { Zap, Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
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
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [])

  const handleTest = useCallback(async () => {
    setStatus('testing')
    setError('')
    try {
      const result = await testCLIModel(modelId)
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
      setError((err as Error).message)
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
        ) : status === 'error' ? (
          <Zap className="h-4 w-4 text-red-500" />
        ) : (
          <Zap className="h-4 w-4" />
        )}
      </Button>
      {status === 'success' && (
        <span className="text-xs text-green-600 dark:text-green-400">{durationMs}ms</span>
      )}
      {status === 'error' && (
        <div className="absolute left-0 right-0 top-full mt-1 text-sm text-red-600 dark:text-red-400 bg-red-500/10 rounded p-2 break-words z-10">
          {error}
        </div>
      )}
    </>
  )
}
