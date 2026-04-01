import { cn } from '@/lib/utils'

const TOOL_COLORS: Record<string, string> = {
  Bash: 'bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300',
  Read: 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300',
  Edit: 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
  Write: 'bg-purple-100 text-purple-800 dark:bg-purple-900/40 dark:text-purple-300',
  Grep: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/40 dark:text-cyan-300',
  Glob: 'bg-teal-100 text-teal-800 dark:bg-teal-900/40 dark:text-teal-300',
  Task: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900/40 dark:text-indigo-300',
  Agent: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900/40 dark:text-indigo-300',
  TaskResult: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300',
  AgentResult: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300',
  WebFetch: 'bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-300',
  WebSearch: 'bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-300',
  rate_limit: 'bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-300',
  TodoWrite: 'bg-pink-100 text-pink-800 dark:bg-pink-900/40 dark:text-pink-300',
  Skill: 'bg-violet-100 text-violet-800 dark:bg-violet-900/40 dark:text-violet-300',
}

const DEFAULT_TOOL_COLOR = 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300'

export function parseToolName(message: string): { toolName: string | null; rest: string } {
  if (!message) return { toolName: null, rest: '' }
  const match = message.match(/^\[(\w+)\]\s*(.*)$/s)
  if (!match) return { toolName: null, rest: message }
  return { toolName: match[1], rest: match[2] }
}

export function ToolBadge({ name }: { name: string }) {
  const colorClass = TOOL_COLORS[name] ?? DEFAULT_TOOL_COLOR
  return (
    <span className={cn('inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold mr-1.5 shrink-0', colorClass)}>
      {name}
    </span>
  )
}

interface LogMessageProps {
  message: string
  variant?: 'compact' | 'full'
  className?: string
}

export function LogMessage({ message, variant = 'compact', className }: LogMessageProps) {
  const { toolName, rest } = parseToolName(message)

  return (
    <div
      className={cn(
        variant === 'compact'
          ? 'px-2 py-1 rounded-md border bg-muted/30 font-mono text-xs text-foreground/90'
          : 'p-3 rounded-lg border bg-muted/30 font-mono text-sm text-foreground/90',
        'whitespace-pre-wrap break-words',
        className,
      )}
    >
      {toolName && <ToolBadge name={toolName} />}
      {rest}
    </div>
  )
}
