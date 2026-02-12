import { CheckCircle, XCircle } from 'lucide-react'
import { createElement } from 'react'

export function resultColor(result?: string | null): string {
  if (result === 'pass') return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200'
  if (result === 'fail') return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
  if (result === 'skipped') return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200'
  return ''
}

export function ResultIcon({ result }: { result?: string | null }) {
  if (result === 'pass') return createElement(CheckCircle, { className: 'h-3 w-3' })
  if (result === 'fail') return createElement(XCircle, { className: 'h-3 w-3' })
  return null
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  const mins = Math.floor(seconds / 60)
  const secs = seconds % 60
  return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`
}
