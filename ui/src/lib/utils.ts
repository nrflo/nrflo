import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatDate(date: string | Date): string {
  const d = typeof date === 'string' ? new Date(date) : date
  return d.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

export function formatDateTime(date: string | Date): string {
  const d = typeof date === 'string' ? new Date(date) : date
  return d.toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function formatRelativeTime(date: string | Date): string {
  const d = typeof date === 'string' ? new Date(date) : date
  const now = new Date()
  const diffMs = now.getTime() - d.getTime()
  const diffSecs = Math.floor(diffMs / 1000)
  const diffMins = Math.floor(diffSecs / 60)
  const diffHours = Math.floor(diffMins / 60)
  const diffDays = Math.floor(diffHours / 24)

  if (diffSecs < 60) return 'just now'
  if (diffMins < 60) return `${diffMins}m ago`
  if (diffHours < 24) return `${diffHours}h ago`
  if (diffDays < 7) return `${diffDays}d ago`
  return formatDate(d)
}

export function formatElapsedTime(startDate: string | Date, endDate?: string | Date): string {
  const start = typeof startDate === 'string' ? new Date(startDate) : startDate
  // Handle empty/invalid endDate by falling back to current time
  let end: Date
  if (endDate) {
    const parsed = typeof endDate === 'string' ? new Date(endDate) : endDate
    end = isNaN(parsed.getTime()) ? new Date() : parsed
  } else {
    end = new Date()
  }

  const diffMs = end.getTime() - start.getTime()
  if (diffMs < 0 || isNaN(diffMs)) return '0s'

  const diffSecs = Math.floor(diffMs / 1000)
  const diffMins = Math.floor(diffSecs / 60)
  const diffHours = Math.floor(diffMins / 60)

  if (diffHours > 0) {
    const mins = diffMins % 60
    return mins > 0 ? `${diffHours}h ${mins}m` : `${diffHours}h`
  }
  if (diffMins > 0) {
    const secs = diffSecs % 60
    return secs > 0 ? `${diffMins}m ${secs}s` : `${diffMins}m`
  }
  return `${diffSecs}s`
}

export function formatTokenCount(n: number): string {
  if (n >= 1_000_000) {
    const m = n / 1_000_000
    return m % 1 === 0 ? `${m}M` : `${m.toFixed(1)}M`
  }
  if (n >= 1_000) {
    const k = n / 1_000
    return k % 1 === 0 ? `${k}K` : `${k.toFixed(1)}K`
  }
  return String(n)
}

export function formatDurationSec(totalSec: number): string {
  const hours = Math.floor(totalSec / 3600)
  const mins = Math.floor((totalSec % 3600) / 60)
  const secs = Math.floor(totalSec % 60)

  if (hours > 0) {
    return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`
  }
  if (mins > 0) {
    return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`
  }
  return `${secs}s`
}

export function capitalize(str: string): string {
  return str.charAt(0).toUpperCase() + str.slice(1)
}

export function statusColor(status: string): string {
  switch (status) {
    case 'open':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
    case 'in_progress':
    case 'running':
      return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200'
    case 'closed':
    case 'completed':
      return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200'
    case 'skipped':
      return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
    case 'pending':
      return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
    case 'error':
    case 'failed':
      return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
    case 'canceled':
      return 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200'
    default:
      return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
  }
}

export function priorityLabel(priority: number): string {
  switch (priority) {
    case 1:
      return 'Critical'
    case 2:
      return 'High'
    case 3:
      return 'Medium'
    case 4:
      return 'Low'
    default:
      return `P${priority}`
  }
}

/** Returns true when context_left is within 15 percentage points of the restart threshold */
export function isNearRestartThreshold(contextLeft: number, threshold: number): boolean {
  return contextLeft <= threshold + 15
}

export function contextLeftColor(contextLeft: number): string {
  if (contextLeft <= 25) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  if (contextLeft <= 50) return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
}

export function issueTypeColor(type: string): string {
  switch (type) {
    case 'bug':
      return 'text-red-600 dark:text-red-400'
    case 'feature':
      return 'text-purple-600 dark:text-purple-400'
    case 'task':
      return 'text-blue-600 dark:text-blue-400'
    case 'epic':
      return 'text-green-600 dark:text-green-400'
    default:
      return 'text-gray-600 dark:text-gray-400'
  }
}
