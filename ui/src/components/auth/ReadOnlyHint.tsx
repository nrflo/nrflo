import { AlertTriangle } from 'lucide-react'

export function ReadOnlyHint() {
  return (
    <div className="flex items-center gap-2 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-400">
      <AlertTriangle className="h-4 w-4 shrink-0" />
      <span>Read-only — admin required to make changes.</span>
    </div>
  )
}
