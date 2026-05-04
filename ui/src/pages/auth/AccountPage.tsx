import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Input } from '@/components/ui/Input'
import { Button } from '@/components/ui/Button'
import { useAuthStore } from '@/stores/authStore'
import { ApiError } from '@/api/client'
import { changePassword } from '@/api/auth'

export function AccountPage() {
  const [searchParams] = useSearchParams()
  const isForced = searchParams.get('force') === '1'
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [isPending, setIsPending] = useState(false)
  const navigate = useNavigate()
  const refresh = useAuthStore((s) => s.refresh)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (next !== confirm) {
      setError('Passwords do not match')
      return
    }
    setError(null)
    setIsPending(true)
    try {
      await changePassword(current, next)
      await refresh()
      navigate('/', { replace: true })
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Failed to change password')
      }
    } finally {
      setIsPending(false)
    }
  }

  return (
    <div className="max-w-sm mx-auto space-y-6 py-8">
      {isForced && (
        <div className="bg-amber-500/10 border border-amber-500/30 rounded-lg p-4 text-sm text-amber-700 dark:text-amber-300">
          You must change your password before continuing.
        </div>
      )}
      <h1 className="text-2xl font-bold">Change Password</h1>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="space-y-2">
          <label htmlFor="current" className="text-sm font-medium">
            Current Password
          </label>
          <Input
            id="current"
            type="password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            autoComplete="current-password"
            required
          />
        </div>
        <div className="space-y-2">
          <label htmlFor="new-password" className="text-sm font-medium">
            New Password
          </label>
          <Input
            id="new-password"
            type="password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            autoComplete="new-password"
            required
          />
        </div>
        <div className="space-y-2">
          <label htmlFor="confirm" className="text-sm font-medium">
            Confirm Password
          </label>
          <Input
            id="confirm"
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            autoComplete="new-password"
            required
          />
        </div>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <Button type="submit" className="w-full" disabled={isPending}>
          {isPending ? 'Changing…' : 'Change Password'}
        </Button>
      </form>
    </div>
  )
}
