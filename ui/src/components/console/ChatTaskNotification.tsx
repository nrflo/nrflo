import { parseTaskNotification } from './taskNotification'

interface ChatTaskNotificationProps {
  content: string
}

// Collapsed muted card for category='task_notification' rows — an async
// task result the console shouldn't dump inline. Same <details> idiom as
// ChatThinking / MessageTableRow's payload reveal.
export function ChatTaskNotification({ content }: ChatTaskNotificationProps) {
  const parsed = parseTaskNotification(content)

  if (!parsed) {
    return (
      <div className="rounded-md border border-border bg-muted/20 px-3 py-2 text-xs whitespace-pre-wrap break-words font-mono">
        {content}
      </div>
    )
  }

  const { taskId, status, summary, result } = parsed

  return (
    <details className="rounded-md border border-border bg-muted/30 px-3 py-2">
      <summary className="cursor-pointer select-none text-xs text-muted-foreground">
        <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold mr-1.5 bg-indigo-100 text-indigo-800 dark:bg-indigo-900/40 dark:text-indigo-300">
          Task
        </span>
        task {taskId ?? 'unknown'} · {status ?? 'unknown'}
        {summary ? ` — ${summary}` : ''}
      </summary>
      {result && (
        <pre className="text-[10px] mt-1.5 p-1 bg-muted rounded overflow-auto whitespace-pre-wrap break-words not-prose">
          {result}
        </pre>
      )}
    </details>
  )
}
