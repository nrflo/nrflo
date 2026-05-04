import { Link } from 'react-router-dom'
import { useMustChangePassword } from '@/stores/authStore'

export function MustChangePasswordBanner() {
  const mustChange = useMustChangePassword()
  if (!mustChange) return null

  return (
    <div className="bg-amber-500/10 dark:bg-amber-500/20 border-b border-amber-500/30 px-4 py-2 text-sm text-amber-700 dark:text-amber-300 flex items-center gap-2">
      <span>Your password must be changed before continuing.</span>
      <Link to="/account?force=1" className="font-medium underline hover:no-underline">
        Change it now
      </Link>
    </div>
  )
}
